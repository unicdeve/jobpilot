//go:build integration

package httpapi

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/version"
)

// buildDaemon builds cmd/jobpilot as the given version, as a release does.
func buildDaemon(t *testing.T, as string) string {
	t.Helper()
	name := "jobpilot"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", out,
		"-ldflags", "-X github.com/unicdev/jobpilot/internal/version.Version="+as,
		"github.com/unicdev/jobpilot/cmd/jobpilot")
	built, err := cmd.CombinedOutput()
	require.NoError(t, err, string(built))
	return out
}

// asVersion has this test's client say it is the given version.
func asVersion(t *testing.T, v string) {
	t.Helper()
	was := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = was })
}

type notices struct {
	mu   sync.Mutex
	said []string
}

func (n *notices) add(s string) { n.mu.Lock(); n.said = append(n.said, s); n.mu.Unlock() }

func (n *notices) all() string { n.mu.Lock(); defer n.mu.Unlock(); return strings.Join(n.said, "\n") }

// TestEnsureDaemon_ReplacesAnOlderDaemon runs real daemons, as an upgrade
// meets them: a newer client stops the older daemon, waits for its process
// to go, and starts its own; an older client uses a newer daemon and says
// so (ADR-0029 §4).
func TestEnsureDaemon_ReplacesAnOlderDaemon(t *testing.T) {
	useDataDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t.Cleanup(func() { _ = StopDaemon(context.Background()) })

	older := buildDaemon(t, "1.0.0")
	newer := buildDaemon(t, "1.1.0")
	path := func(p string) func() (string, error) { return func() (string, error) { return p, nil } }

	asVersion(t, "1.0.0")
	var said notices
	client, err := EnsureDaemon(ctx, path(older), said.add)
	require.NoError(t, err)
	running, err := client.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", running)
	assert.Empty(t, said.all())

	version.Version = "1.1.0"
	client, err = EnsureDaemon(ctx, path(newer), said.add)
	require.NoError(t, err)
	running, err = client.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", running, "the older daemon was replaced")
	assert.Contains(t, said.all(), "replacing the running daemon, JobPilot 1.0.0, with 1.1.0")

	version.Version = "1.0.5"
	said = notices{}
	client, err = EnsureDaemon(ctx, func() (string, error) {
		t.Error("an older client starts nothing")
		return older, nil
	}, said.add)
	require.NoError(t, err)
	running, err = client.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", running)
	assert.Contains(t, said.all(), "this is JobPilot 1.0.5, which is older than the running daemon's 1.1.0")

	require.NoError(t, StopDaemon(ctx))
	stillRunning, err := system.DaemonRunning()
	require.NoError(t, err)
	assert.False(t, stillRunning, "stopping waits for the process to go")
	_, found := FindDaemon(ctx)
	assert.False(t, found)
}
