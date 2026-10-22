package state

import (
	"net/netip"
	"testing"

	"github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/views"
)

func TestBuildPeerMapByNetwork(t *testing.T) {
	user := &types.User{Name: "default"}
	user.ID = 1
	uid := user.ID
	addr := func(s string) *netip.Addr {
		parsed := netip.MustParseAddr(s)

		return &parsed
	}

	nodes := []types.Node{
		{ID: 1, NetworkID: 1, GivenName: "a", Hostname: "a", IPv4: addr("100.64.0.1"), UserID: &uid, User: user},
		{ID: 2, NetworkID: 1, GivenName: "b", Hostname: "b", IPv4: addr("100.64.0.2"), UserID: &uid, User: user},
		{ID: 3, NetworkID: 2, GivenName: "c", Hostname: "c", IPv4: addr("100.64.0.3"), UserID: &uid, User: user},
		{ID: 4, NetworkID: 2, GivenName: "d", Hostname: "d", IPv4: addr("100.64.0.4"), UserID: &uid, User: user},
	}

	nodeViews := make([]types.NodeView, len(nodes))
	for i := range nodes {
		nodeViews[i] = nodes[i].View()
	}

	allowAll := []byte(`{"acls":[{"action":"accept","src":["*"],"dst":["*:*"]}]}`)

	manager, err := policy.NewPolicyManager(allowAll, []types.User{*user}, views.SliceOf(nodeViews))
	require.NoError(t, err)

	peers := buildPeerMapByNetwork(manager, nodeViews)

	require.Equal(t, []types.NodeID{2}, peers[1])
	require.Equal(t, []types.NodeID{1}, peers[2])
	require.Equal(t, []types.NodeID{4}, peers[3])
	require.Equal(t, []types.NodeID{3}, peers[4])
}
