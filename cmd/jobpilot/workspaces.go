package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

type workspaceRuntime struct {
	db       *sqlite.DB
	services httpapi.Services
	handler  http.Handler
}

type workspaceManager struct {
	mu                sync.Mutex
	ctx               context.Context
	stop              context.CancelFunc
	catalog           *system.WorkspaceStore
	runtimes          map[string]*workspaceRuntime
	desktopExecutable string
}

func newWorkspaceManager(
	ctx context.Context,
	stop context.CancelFunc,
	catalog *system.WorkspaceStore,
) (*workspaceManager, error) {
	m := &workspaceManager{
		ctx:      ctx,
		stop:     stop,
		catalog:  catalog,
		runtimes: map[string]*workspaceRuntime{},
	}
	workspaces, _ := catalog.List()
	active, err := catalog.Active()
	if err != nil {
		return nil, err
	}
	if _, err := m.open(active); err != nil {
		m.close()
		return nil, fmt.Errorf("opening workspace %q: %w", active.Name, err)
	}
	for _, workspace := range workspaces {
		if workspace.ID == active.ID {
			continue
		}
		if _, err := m.open(workspace); err != nil {
			log.Printf("workspace %s could not be opened and will be retried when selected: %v", workspace.ID, err)
		}
	}
	return m, nil
}

func (m *workspaceManager) RegisterDesktop(ctx context.Context, executable string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, runtime := range m.runtimes {
		if err := runtime.services.Notifications.RegisterDesktop(ctx, executable); err != nil {
			return fmt.Errorf("registering desktop for workspace %s: %w", id, err)
		}
	}
	m.desktopExecutable = executable
	return nil
}

func (m *workspaceManager) Resolve(workspace system.Workspace) (http.Handler, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runtime := m.runtimes[workspace.ID]; runtime != nil {
		return runtime.handler, nil
	}
	runtime, err := m.open(workspace)
	if err != nil {
		return nil, err
	}
	return runtime.handler, nil
}

func (m *workspaceManager) open(workspace system.Workspace) (*workspaceRuntime, error) {
	db, err := sqlite.NewDB(m.catalog.DBPath(workspace))
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.InitializeSchema(m.ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing database schema: %w", err)
	}

	namespace := workspace.ID
	if workspace.Legacy {
		namespace = ""
	}
	services, err := wireServices(
		db,
		m.catalog.Dir(workspace),
		namespace,
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("wiring services: %w", err)
	}
	services.Stop = m.stop
	if m.desktopExecutable != "" {
		if err := services.Notifications.RegisterDesktop(m.ctx, m.desktopExecutable); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("registering desktop: %w", err)
		}
	}
	// Before anything can start a run: one still open now was cut off by a
	// daemon that stopped without closing it.
	if closed, err := services.Activity.CloseUnfinishedRuns(m.ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("closing unfinished runs: %w", err)
	} else if closed > 0 {
		log.Printf("workspace %s: closed %d runs a stopped daemon left open", workspace.ID, closed)
	}
	services.Scouts.Start(m.ctx)
	runtime := &workspaceRuntime{
		db:       db,
		services: services,
		handler:  httpapi.NewHandler(services),
	}
	m.runtimes[workspace.ID] = runtime

	go func() {
		read, readErr := services.Discovery.ReadStoredSignals(m.ctx)
		switch {
		case readErr != nil && m.ctx.Err() == nil:
			log.Printf("workspace %s: reading stored jobs again: %v", workspace.ID, readErr)
		case read > 0:
			log.Printf("workspace %s: read %d stored jobs again", workspace.ID, read)
		}
	}()
	return runtime, nil
}

func (m *workspaceManager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, runtime := range m.runtimes {
		runtime.services.Scouts.Wait()
		runtime.services.Notifications.Wait()
		if err := runtime.db.Close(); err != nil {
			log.Printf("workspace %s: error closing database: %v", id, err)
		}
	}
}
