package integration

import (
	"fmt"
	"testing"
	"time"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/integration/hsic"
	"github.com/juanfont/headscale/integration/integrationutil"
	"github.com/juanfont/headscale/integration/tsic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
)

// Ports match inferoute-node/pkg/cluster/engine.go TailnetPolicy.
// This repo does not import that module, so a drift there will not fail this test.
func TestClusterTailnetGrants(t *testing.T) {
	IntegrationSkip(t)

	const (
		vllmPort     = 8000
		llamaPort    = 8081
		masterPort   = 29501
		ncclSample   = 40000
		ephemeralMin = 32768
		ephemeralMax = 60999
	)

	allowed := []int{vllmPort, llamaPort, masterPort, ncclSample}
	blocked := []int{22, 80, 443, 445, 3389, 5432, 6379, ephemeralMin - 1, ephemeralMax + 1}

	pol := &policyv2.Policy{
		Grants: []policyv2.Grant{{
			Sources:      policyv2.Aliases{new(policyv2.AutoGroupMember)},
			Destinations: policyv2.Aliases{new(policyv2.AutoGroupMember)},
			InternetProtocols: []policyv2.ProtocolPort{
				{Protocol: policyv2.ProtocolNameTCP, Ports: []tailcfg.PortRange{{First: vllmPort, Last: vllmPort}}},
				{Protocol: policyv2.ProtocolNameTCP, Ports: []tailcfg.PortRange{{First: llamaPort, Last: llamaPort}}},
				{Protocol: policyv2.ProtocolNameTCP, Ports: []tailcfg.PortRange{{First: masterPort, Last: masterPort}}},
				{Protocol: policyv2.ProtocolNameTCP, Ports: []tailcfg.PortRange{{First: ephemeralMin, Last: ephemeralMax}}},
				{Protocol: policyv2.ProtocolNameICMP, Ports: []tailcfg.PortRange{tailcfg.PortRangeAny}},
			},
		}},
	}

	scenario, err := NewScenario(ScenarioSpec{
		NodesPerUser: 1,
		Users:        []string{"user1", "user2"},
	})
	require.NoError(t, err)
	defer scenario.ShutdownAssertNoPanics(t)

	err = scenario.CreateHeadscaleEnv(
		[]tsic.Option{
			tsic.WithNetfilter("off"),
			tsic.WithPackages("curl", "python3"),
		},
		hsic.WithACLPolicy(pol),
		hsic.WithPolicyMode(types.PolicyModeDB),
		hsic.WithTestName("cluster-tailnet-grants"),
	)
	require.NoError(t, err)

	err = scenario.WaitForTailscaleSync()
	require.NoError(t, err)

	user1, err := scenario.ListTailscaleClients("user1")
	require.NoError(t, err)
	user2, err := scenario.ListTailscaleClients("user2")
	require.NoError(t, err)
	require.Len(t, user1, 1)
	require.Len(t, user2, 1)

	all := append(append([]int{}, allowed...), blocked...)
	for _, client := range []TailscaleClient{user1[0], user2[0]} {
		startClusterHTTP(t, client, all)
	}

	pairs := [][2]TailscaleClient{{user1[0], user2[0]}, {user2[0], user1[0]}}
	for _, pair := range pairs {
		from, to := pair[0], pair[1]
		fqdn, err := to.FQDN()
		require.NoError(t, err)

		for _, port := range allowed {
			url := fmt.Sprintf("http://%s:%d/etc/hostname", fqdn, port)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assertCurlDockerHostname(c, from, url, "allowed port")
			}, integrationutil.ScaledTimeout(20*time.Second), integrationutil.SlowPoll, "allowed %s", url)
		}
		for _, port := range blocked {
			url := fmt.Sprintf("http://%s:%d/etc/hostname", fqdn, port)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assertCurlFailWithCollect(c, from, url, "blocked port")
			}, integrationutil.ScaledTimeout(20*time.Second), integrationutil.SlowPoll, "blocked %s", url)
		}

		ip := to.MustIPv4().String()
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			// tailscale ping is disco and does not hit the packet filter.
			_, _, err := from.Execute([]string{"ping", "-c", "1", "-W", "2", ip})
			assert.NoError(c, err)
		}, integrationutil.ScaledTimeout(20*time.Second), integrationutil.SlowPoll, "icmp %s", ip)
	}
}

func startClusterHTTP(t *testing.T, client TailscaleClient, ports []int) {
	t.Helper()

	// Subshell so this exec returns. The parent shell does not wait on the servers.
	script := ""
	for _, port := range ports {
		script += fmt.Sprintf(
			"(python3 -m http.server --directory / --bind :: %d >/tmp/http-%d.log 2>&1 &)\n",
			port, port,
		)
	}

	_, stderr, err := client.Execute([]string{"sh", "-c", script})
	require.NoError(t, err, stderr)

	for _, port := range ports {
		url := fmt.Sprintf("http://127.0.0.1:%d/etc/hostname", port)
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			assertCurlDockerHostname(c, client, url, "listener up")
		}, integrationutil.ScaledTimeout(20*time.Second), integrationutil.SlowPoll, "listen %s", url)
	}
}
