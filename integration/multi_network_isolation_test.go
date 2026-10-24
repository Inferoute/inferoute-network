package integration

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/integration/hsic"
	"github.com/juanfont/headscale/integration/integrationutil"
	"github.com/juanfont/headscale/integration/tsic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTwoIsolatedNetworks is the cluster flow without Inferoute: two tailnets
// on one process, invite-by-key, ping inside, no peers across.
//
// shortcut: one reusable key per network. RunTailscaleUp logs every client in
// that bucket in with the same key. Product invites are single-use.
func TestTwoIsolatedNetworks(t *testing.T) {
	IntegrationSkip(t)

	scenario, err := NewScenario(ScenarioSpec{})
	require.NoError(t, err)
	t.Cleanup(func() {
		scenario.ShutdownAssertNoPanics(t)
	})

	headscale, err := scenario.Headscale(hsic.WithTestName("twonet"))
	require.NoError(t, err)
	require.NoError(t, headscale.WaitForRunning())

	names := []string{"cluster-a", "cluster-b"}
	for _, name := range names {
		_, err = headscale.CreateNetwork(name)
		require.NoError(t, err)

		scenario.GetOrCreateUser(name)

		err = scenario.CreateTailscaleNodesInUser(
			name,
			"head",
			2,
			tsic.WithNetwork(scenario.networks[scenario.testDefaultNetwork]),
		)
		require.NoError(t, err)

		key, err := headscale.CreateAuthKeyWithOptions(hsic.AuthKeyOptions{
			Network:  name,
			Reusable: true,
		})
		require.NoError(t, err)

		err = scenario.RunTailscaleUp(name, headscale.GetEndpoint(), key.Key)
		require.NoError(t, err)
	}

	err = scenario.WaitForTailscaleSyncPerUser(
		integrationutil.ScaledTimeout(60*time.Second),
		500*time.Millisecond,
	)
	require.NoError(t, err)

	for _, name := range names {
		clients, err := scenario.GetClients(name)
		require.NoError(t, err)

		for _, client := range clients {
			status, err := client.Status()
			require.NoError(t, err)
			assert.Len(t, status.Peer, 1, "%s should see only its roommate", client.Hostname())
		}

		ips, err := scenario.GetIPs(name)
		require.NoError(t, err)

		addrs := make([]string, 0, len(ips))
		for _, ip := range ips {
			addrs = append(addrs, ip.String())
		}

		assertPingAll(t, clients, addrs)
	}

	aClients, err := scenario.GetClients("cluster-a")
	require.NoError(t, err)
	bIPs, err := scenario.GetIPs("cluster-b")
	require.NoError(t, err)

	var other string

	for _, ip := range bIPs {
		if ip.Is4() {
			other = ip.String()

			break
		}
	}

	require.NotEmpty(t, other)
	require.Error(t, aClients[0].Ping(other))
}
