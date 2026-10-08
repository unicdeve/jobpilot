package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

const (
	workspaceOneID = "11111111-1111-4111-8111-111111111111"
	workspaceTwoID = "22222222-2222-4222-8222-222222222222"
)

type fakeWorkspaceCatalog struct {
	workspaces map[string]system.Workspace
	activeID   string
}

func newFakeWorkspaceCatalog() *fakeWorkspaceCatalog {
	return &fakeWorkspaceCatalog{
		workspaces: map[string]system.Workspace{
			workspaceOneID: {ID: workspaceOneID, Name: "Personal", Legacy: true},
			workspaceTwoID: {ID: workspaceTwoID, Name: "Consulting"},
		},
		activeID: workspaceOneID,
	}
}

func (f *fakeWorkspaceCatalog) List() ([]system.Workspace, string) {
	return []system.Workspace{f.workspaces[workspaceOneID], f.workspaces[workspaceTwoID]}, f.activeID
}

func (f *fakeWorkspaceCatalog) Get(id string) (system.Workspace, error) {
	workspace, ok := f.workspaces[id]
	if !ok {
		return system.Workspace{}, system.ErrWorkspaceNotFound
	}
	return workspace, nil
}

func (f *fakeWorkspaceCatalog) Active() (system.Workspace, error) {
	return f.Get(f.activeID)
}

func (f *fakeWorkspaceCatalog) Create(name string) (system.Workspace, error) {
	workspace := system.Workspace{ID: workspaceTwoID, Name: name}
	f.workspaces[workspace.ID] = workspace
	return workspace, nil
}

func (f *fakeWorkspaceCatalog) Rename(id, name string) (system.Workspace, error) {
	workspace, err := f.Get(id)
	if err != nil {
		return system.Workspace{}, err
	}
	workspace.Name = name
	f.workspaces[id] = workspace
	return workspace, nil
}

func (f *fakeWorkspaceCatalog) Activate(id string) (system.Workspace, error) {
	workspace, err := f.Get(id)
	if err != nil {
		return system.Workspace{}, err
	}
	f.activeID = id
	return workspace, nil
}

type fakeWorkspaceResolver struct {
	err               error
	desktopExecutable string
}

func (f *fakeWorkspaceResolver) RegisterDesktop(_ context.Context, executable string) error {
	if f.err != nil {
		return f.err
	}
	f.desktopExecutable = executable
	return nil
}

func (f *fakeWorkspaceResolver) Resolve(workspace system.Workspace) (http.Handler, error) {
	if f.err != nil {
		return nil, f.err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(workspace.ID))
	}), nil
}

func TestWorkspaceHandler_DispatchesToSelectedWorkspace(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{})

	request := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	request.Header.Set(WorkspaceHeader, workspaceTwoID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, workspaceTwoID, response.Body.String())
}

func TestWorkspaceHandler_UsesActiveWorkspaceWhenHeaderIsAbsent(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/settings", nil))

	assert.Equal(t, workspaceOneID, response.Body.String())
}

func TestWorkspaceHandler_RejectsUnknownWorkspace(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{})
	request := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	request.Header.Set(WorkspaceHeader, "33333333-3333-4333-8333-333333333333")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestWorkspaceHandler_ManagesCatalog(t *testing.T) {
	catalog := newFakeWorkspaceCatalog()
	handler := NewWorkspaceHandler(catalog, &fakeWorkspaceResolver{})

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"Side project"}`)))
	require.Equal(t, http.StatusCreated, create.Code)
	assert.Contains(t, create.Body.String(), "Side project")

	rename := httptest.NewRecorder()
	handler.ServeHTTP(rename, httptest.NewRequest(http.MethodPut, "/v1/workspaces/"+workspaceTwoID, strings.NewReader(`{"name":"Consulting"}`)))
	require.Equal(t, http.StatusOK, rename.Code)
	assert.Contains(t, rename.Body.String(), "Consulting")

	activate := httptest.NewRecorder()
	handler.ServeHTTP(activate, httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+workspaceTwoID+"/activate", nil))
	require.Equal(t, http.StatusOK, activate.Code)
	assert.Equal(t, workspaceTwoID, catalog.activeID)
}

func TestWorkspaceHandler_ReportsResolverFailure(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{err: errors.New("open failed")})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/settings", nil))

	assert.Equal(t, http.StatusInternalServerError, response.Code)
}

func TestWorkspaceHandler_RegistersDesktopForEveryWorkspace(t *testing.T) {
	resolver := &fakeWorkspaceResolver{}
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), resolver)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/desktop", strings.NewReader(`{"executable":"/opt/jobpilot"}`)))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "/opt/jobpilot", resolver.desktopExecutable)
}

func TestWorkspaceHandler_AllowsDesktopOriginOnCatalog(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{})
	request := httptest.NewRequest(http.MethodGet, "/v1/workspaces", nil)
	request.Header.Set("Origin", "tauri://localhost")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "tauri://localhost", response.Header().Get("Access-Control-Allow-Origin"))
}

func TestWorkspaceHandler_PreflightAllowsWorkspaceHeader(t *testing.T) {
	handler := NewWorkspaceHandler(newFakeWorkspaceCatalog(), &fakeWorkspaceResolver{})
	request := httptest.NewRequest(http.MethodOptions, "/v1/profile", nil)
	request.Header.Set("Origin", "tauri://localhost")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	request.Header.Set("Access-Control-Request-Headers", WorkspaceHeader)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Contains(t, response.Header().Get("Access-Control-Allow-Headers"), WorkspaceHeader)
}
