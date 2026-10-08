package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	workspaceCatalogName = "workspaces.json"
	workspacesDirName    = "workspaces"
	maxWorkspaceName     = 80
)

var (
	// ErrWorkspaceNotFound means an identifier is not registered locally.
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrWorkspaceNameUsed means another workspace has the same display name.
	ErrWorkspaceNameUsed = errors.New("workspace name is already used")
)

// Workspace is one isolated set of JobPilot data on this installation.
// Legacy is true only for the workspace that adopts the pre-workspace data
// directory in place.
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Legacy    bool      `json:"legacy,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type workspaceCatalog struct {
	ActiveID   string      `json:"active_id"`
	Workspaces []Workspace `json:"workspaces"`
}

// WorkspaceStore owns the installation-level workspace catalog.
type WorkspaceStore struct {
	mu      sync.RWMutex
	rootDir string
	path    string
	catalog workspaceCatalog
}

// OpenWorkspaceStore loads the workspace catalog, creating one that adopts
// the existing installation in place when this is the first workspace-aware
// version to run.
func OpenWorkspaceStore() (*WorkspaceStore, error) {
	rootDir, err := EnsureDataDir()
	if err != nil {
		return nil, err
	}
	s := &WorkspaceStore{
		rootDir: rootDir,
		path:    filepath.Join(rootDir, workspaceCatalogName),
	}
	if err := s.loadOrCreate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *WorkspaceStore) loadOrCreate() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		now := time.Now().UTC()
		first := Workspace{
			ID:        uuid.NewString(),
			Name:      "Personal",
			Legacy:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.catalog = workspaceCatalog{ActiveID: first.ID, Workspaces: []Workspace{first}}
		return s.writeLocked()
	}
	if err != nil {
		return fmt.Errorf("reading workspace catalog: %w", err)
	}
	if err := json.Unmarshal(data, &s.catalog); err != nil {
		return fmt.Errorf("parsing workspace catalog: %w", err)
	}
	return s.validateCatalog()
}

func (s *WorkspaceStore) validateCatalog() error {
	if len(s.catalog.Workspaces) == 0 {
		return errors.New("workspace catalog has no workspaces")
	}
	seenIDs := map[string]bool{}
	seenNames := map[string]bool{}
	legacyCount := 0
	for _, workspace := range s.catalog.Workspaces {
		if err := validateWorkspaceID(workspace.ID); err != nil {
			return err
		}
		if seenIDs[workspace.ID] {
			return fmt.Errorf("workspace catalog repeats id %q", workspace.ID)
		}
		seenIDs[workspace.ID] = true
		name, err := normalizeWorkspaceName(workspace.Name)
		if err != nil {
			return fmt.Errorf("workspace %s: %w", workspace.ID, err)
		}
		folded := strings.ToLower(name)
		if seenNames[folded] {
			return fmt.Errorf("workspace catalog repeats name %q", name)
		}
		seenNames[folded] = true
		if workspace.Legacy {
			legacyCount++
		}
	}
	if legacyCount > 1 {
		return errors.New("workspace catalog has more than one legacy workspace")
	}
	if !seenIDs[s.catalog.ActiveID] {
		return fmt.Errorf("active workspace %q: %w", s.catalog.ActiveID, ErrWorkspaceNotFound)
	}
	return nil
}

// List returns every workspace in stable creation order and the active ID.
func (s *WorkspaceStore) List() ([]Workspace, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.catalog.Workspaces), s.catalog.ActiveID
}

// Get returns a registered workspace.
func (s *WorkspaceStore) Get(id string) (Workspace, error) {
	if err := validateWorkspaceID(id); err != nil {
		return Workspace{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, workspace := range s.catalog.Workspaces {
		if workspace.ID == id {
			return workspace, nil
		}
	}
	return Workspace{}, fmt.Errorf("workspace %s: %w", id, ErrWorkspaceNotFound)
}

// Active returns the installation's selected workspace.
func (s *WorkspaceStore) Active() (Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, workspace := range s.catalog.Workspaces {
		if workspace.ID == s.catalog.ActiveID {
			return workspace, nil
		}
	}
	return Workspace{}, fmt.Errorf("active workspace %s: %w", s.catalog.ActiveID, ErrWorkspaceNotFound)
}

// Create adds a new empty workspace and creates its directory.
func (s *WorkspaceStore) Create(name string) (Workspace, error) {
	name, err := normalizeWorkspaceName(name)
	if err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameUsedLocked(name, "") {
		return Workspace{}, fmt.Errorf("workspace %q: %w", name, ErrWorkspaceNameUsed)
	}
	now := time.Now().UTC()
	workspace := Workspace{ID: uuid.NewString(), Name: name, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(s.rootDir, workspacesDirName, workspace.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Workspace{}, fmt.Errorf("creating workspace directory: %w", err)
	}
	s.catalog.Workspaces = append(s.catalog.Workspaces, workspace)
	if err := s.writeLocked(); err != nil {
		s.catalog.Workspaces = s.catalog.Workspaces[:len(s.catalog.Workspaces)-1]
		return Workspace{}, err
	}
	return workspace, nil
}

// Rename changes a workspace's display name.
func (s *WorkspaceStore) Rename(id, name string) (Workspace, error) {
	if err := validateWorkspaceID(id); err != nil {
		return Workspace{}, err
	}
	name, err := normalizeWorkspaceName(name)
	if err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameUsedLocked(name, id) {
		return Workspace{}, fmt.Errorf("workspace %q: %w", name, ErrWorkspaceNameUsed)
	}
	for i := range s.catalog.Workspaces {
		if s.catalog.Workspaces[i].ID != id {
			continue
		}
		old := s.catalog.Workspaces[i]
		s.catalog.Workspaces[i].Name = name
		s.catalog.Workspaces[i].UpdatedAt = time.Now().UTC()
		if err := s.writeLocked(); err != nil {
			s.catalog.Workspaces[i] = old
			return Workspace{}, err
		}
		return s.catalog.Workspaces[i], nil
	}
	return Workspace{}, fmt.Errorf("workspace %s: %w", id, ErrWorkspaceNotFound)
}

// Activate selects the workspace used by clients that do not send an
// explicit workspace header.
func (s *WorkspaceStore) Activate(id string) (Workspace, error) {
	if err := validateWorkspaceID(id); err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, workspace := range s.catalog.Workspaces {
		if workspace.ID != id {
			continue
		}
		old := s.catalog.ActiveID
		s.catalog.ActiveID = id
		if err := s.writeLocked(); err != nil {
			s.catalog.ActiveID = old
			return Workspace{}, err
		}
		return workspace, nil
	}
	return Workspace{}, fmt.Errorf("workspace %s: %w", id, ErrWorkspaceNotFound)
}

// Dir returns the workspace's isolated directory. The legacy workspace uses
// the installation root so existing SQLite sidecars and browser data do not
// have to be moved.
func (s *WorkspaceStore) Dir(workspace Workspace) string {
	if workspace.Legacy {
		return s.rootDir
	}
	return filepath.Join(s.rootDir, workspacesDirName, workspace.ID)
}

// DBPath returns the SQLite file owned by workspace.
func (s *WorkspaceStore) DBPath(workspace Workspace) string {
	return filepath.Join(s.Dir(workspace), dbFileName)
}

func (s *WorkspaceStore) nameUsedLocked(name, exceptID string) bool {
	for _, workspace := range s.catalog.Workspaces {
		if workspace.ID != exceptID && strings.EqualFold(workspace.Name, name) {
			return true
		}
	}
	return false
}

func (s *WorkspaceStore) writeLocked() error {
	data, err := json.MarshalIndent(s.catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding workspace catalog: %w", err)
	}
	tmp, err := os.CreateTemp(s.rootDir, ".workspaces-*.json")
	if err != nil {
		return fmt.Errorf("creating workspace catalog temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("securing workspace catalog temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing workspace catalog: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing workspace catalog: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing workspace catalog: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replacing workspace catalog: %w", err)
	}
	return nil
}

func validateWorkspaceID(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != strings.ToLower(id) {
		return fmt.Errorf("workspace id must be a canonical uuid, got %q", id)
	}
	return nil
}

func normalizeWorkspaceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", errors.New("workspace name cannot be empty")
	case len([]rune(name)) > maxWorkspaceName:
		return "", fmt.Errorf("workspace name cannot be longer than %d characters", maxWorkspaceName)
	case strings.ContainsAny(name, "\r\n\x00"):
		return "", errors.New("workspace name cannot contain control characters")
	default:
		return name, nil
	}
}
