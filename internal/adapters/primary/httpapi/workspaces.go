package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

// WorkspaceHeader selects the isolated local workspace a request belongs to.
const WorkspaceHeader = "X-JobPilot-Workspace"

// WorkspaceCatalog is the installation-level workspace metadata used by the
// HTTP boundary. Candidate and job-search data never enters this catalog.
type WorkspaceCatalog interface {
	List() ([]system.Workspace, string)
	Get(id string) (system.Workspace, error)
	Active() (system.Workspace, error)
	Create(name string) (system.Workspace, error)
	Rename(id, name string) (system.Workspace, error)
	Activate(id string) (system.Workspace, error)
}

// WorkspaceResolver returns the already isolated handler for workspace.
type WorkspaceResolver interface {
	Resolve(workspace system.Workspace) (http.Handler, error)
	RegisterDesktop(ctx context.Context, executable string) error
}

// NewWorkspaceHandler adds workspace management and dispatches every
// existing data route to one workspace's ordinary single-workspace handler.
func NewWorkspaceHandler(catalog WorkspaceCatalog, resolver WorkspaceResolver) http.Handler {
	return withCORS(&workspaceHandler{catalog: catalog, resolver: resolver})
}

type workspaceHandler struct {
	catalog  WorkspaceCatalog
	resolver WorkspaceResolver
}

func (h *workspaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/desktop" && r.Method == http.MethodPut {
		handleWorkspaceDesktopRegistration(h.resolver)(w, r)
		return
	}
	if r.URL.Path == "/v1/workspaces" {
		h.handleCollection(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/workspaces/") {
		h.handleMember(w, r)
		return
	}

	workspace, err := h.selected(r)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	handler, err := h.resolver.Resolve(workspace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	handler.ServeHTTP(w, r)
}

func handleWorkspaceDesktopRegistration(resolver WorkspaceResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request desktopRequest
		if err := readJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := resolver.RegisterDesktop(r.Context(), request.Executable); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *workspaceHandler) selected(r *http.Request) (system.Workspace, error) {
	if id := r.Header.Get(WorkspaceHeader); id != "" {
		return h.catalog.Get(id)
	}
	return h.catalog.Active()
}

func (h *workspaceHandler) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspaces, activeID := h.catalog.List()
		writeJSON(w, http.StatusOK, struct {
			Workspaces []system.Workspace `json:"workspaces"`
			ActiveID   string             `json:"active_workspace_id"`
		}{Workspaces: workspaces, ActiveID: activeID})
	case http.MethodPost:
		var request struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid workspace request"))
			return
		}
		workspace, err := h.catalog.Create(request.Name)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspace)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *workspaceHandler) handleMember(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, "/v1/workspaces/")
	id, action, hasAction := strings.Cut(remainder, "/")
	if id == "" || (hasAction && action != "activate") {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodPut && !hasAction:
		var request struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid workspace request"))
			return
		}
		workspace, err := h.catalog.Rename(id, request.Name)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspace)
	case r.Method == http.MethodPost && action == "activate":
		workspace, err := h.catalog.Get(id)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		if _, err := h.resolver.Resolve(workspace); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		workspace, err = h.catalog.Activate(id)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspace)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, system.ErrWorkspaceNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, system.ErrWorkspaceNameUsed):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
