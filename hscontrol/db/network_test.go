package db

import (
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNetworkIsolatesPeers(t *testing.T) {
	db, err := newSQLiteTestDB()
	require.NoError(t, err)

	_, userA, err := db.CreateNetwork("cluster-a")
	require.NoError(t, err)
	_, userB, err := db.CreateNetwork("cluster-b")
	require.NoError(t, err)

	a1 := db.CreateNodeForTest(userA, "a1")
	a2 := db.CreateNodeForTest(userA, "a2")
	b1 := db.CreateNodeForTest(userB, "b1")

	peers, err := db.ListPeers(a1.ID)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, a2.ID, peers[0].ID)

	peers, err = db.ListPeers(b1.ID)
	require.NoError(t, err)
	assert.Empty(t, peers)

	_, _, err = db.CreateNetwork("cluster-a")
	require.ErrorIs(t, err, ErrNetworkNameTaken)

	err = db.DestroyNetwork(userA.NetworkID)
	require.Error(t, err)

	require.NoError(t, db.DeleteNode(a1))
	require.NoError(t, db.DeleteNode(a2))
	require.NoError(t, db.DestroyNetwork(userA.NetworkID))

	_, err = db.GetNetwork(userA.NetworkID)
	require.ErrorIs(t, err, ErrNetworkNotFound)
}

func TestDefaultNetworkRejectsDelete(t *testing.T) {
	db, err := newSQLiteTestDB()
	require.NoError(t, err)

	network, err := db.GetNetworkByName(types.DefaultNetworkName)
	require.NoError(t, err)

	err = db.DestroyNetwork(network.ID)
	require.ErrorIs(t, err, ErrNetworkIsDefault)
}

func TestGetOrCreateUserInNetwork(t *testing.T) {
	db, err := newSQLiteTestDB()
	require.NoError(t, err)

	network, _, err := db.CreateNetwork("cluster-a")
	require.NoError(t, err)

	existing, created, err := db.GetOrCreateUserInNetwork(network.ID, types.DefaultNetworkUser)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, types.DefaultNetworkUser, existing.Name)

	first, created, err := db.GetOrCreateUserInNetwork(network.ID, "alice")
	require.NoError(t, err)
	assert.True(t, created)

	second, created, err := db.GetOrCreateUserInNetwork(network.ID, "alice")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, second.ID)
}

func TestSameUsernameInTwoNetworks(t *testing.T) {
	db, err := newSQLiteTestDB()
	require.NoError(t, err)

	first, _, err := db.CreateNetwork("cluster-a")
	require.NoError(t, err)
	second, _, err := db.CreateNetwork("cluster-b")
	require.NoError(t, err)

	_, err = db.CreateUser(types.User{Name: "alice", NetworkID: first.ID})
	require.NoError(t, err)
	_, err = db.CreateUser(types.User{Name: "alice", NetworkID: second.ID})
	require.NoError(t, err)

	_, err = db.CreateUser(types.User{Name: "alice", NetworkID: first.ID})
	require.Error(t, err)
}
