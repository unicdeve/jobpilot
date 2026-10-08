package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenWorkspaceStore_AdoptsExistingInstallation(t *testing.T) {
	useDataDir(t)
	root, err := EnsureDataDir()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, dbFileName), []byte("existing"), 0o600))

	store, err := OpenWorkspaceStore()
	require.NoError(t, err)
	workspaces, activeID := store.List()
	require.Len(t, workspaces, 1)
	assert.Equal(t, workspaces[0].ID, activeID)
	assert.True(t, workspaces[0].Legacy)
	assert.Equal(t, filepath.Join(root, dbFileName), store.DBPath(workspaces[0]))

	got, err := os.ReadFile(filepath.Join(root, dbFileName))
	require.NoError(t, err)
	assert.Equal(t, []byte("existing"), got)
}

func TestWorkspaceStore_CreateRenameActivateAndReload(t *testing.T) {
	useDataDir(t)
	store, err := OpenWorkspaceStore()
	require.NoError(t, err)

	created, err := store.Create("Contract search")
	require.NoError(t, err)
	assert.False(t, created.Legacy)
	assert.DirExists(t, store.Dir(created))

	renamed, err := store.Rename(created.ID, "Leadership search")
	require.NoError(t, err)
	assert.Equal(t, "Leadership search", renamed.Name)
	_, err = store.Activate(created.ID)
	require.NoError(t, err)

	reloaded, err := OpenWorkspaceStore()
	require.NoError(t, err)
	workspaces, activeID := reloaded.List()
	require.Len(t, workspaces, 2)
	assert.Equal(t, created.ID, activeID)
	assert.Equal(t, "Leadership search", workspaces[1].Name)
}

func TestWorkspaceStore_RejectsInvalidNamesAndIDs(t *testing.T) {
	useDataDir(t)
	store, err := OpenWorkspaceStore()
	require.NoError(t, err)

	_, err = store.Create("   ")
	assert.ErrorContains(t, err, "cannot be empty")
	_, err = store.Create("Personal")
	assert.ErrorIs(t, err, ErrWorkspaceNameUsed)
	_, err = store.Get("../../outside")
	assert.ErrorContains(t, err, "canonical uuid")
}

func TestOpenWorkspaceStore_RejectsCorruptCatalog(t *testing.T) {
	useDataDir(t)
	root, err := EnsureDataDir()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceCatalogName), []byte("{"), 0o600))

	_, err = OpenWorkspaceStore()
	assert.ErrorContains(t, err, "parsing workspace catalog")
}

func TestOpenWorkspaceStore_RejectsUnregisteredActiveWorkspace(t *testing.T) {
	useDataDir(t)
	root, err := EnsureDataDir()
	require.NoError(t, err)
	data, err := json.Marshal(workspaceCatalog{
		ActiveID: "3f31ca7e-6257-4b4f-aa1e-ad77039e346a",
		Workspaces: []Workspace{{
			ID:   "a7bc2891-eb6a-4f23-899d-bb56849594ac",
			Name: "Personal",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, workspaceCatalogName), data, 0o600))

	_, err = OpenWorkspaceStore()
	assert.ErrorIs(t, err, ErrWorkspaceNotFound)
}
