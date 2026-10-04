//go:build !windows

package alarms

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptWorld is a directory of stand-ins for what an alarm's script runs,
// each recording how it was called, so the script itself runs under the
// real /bin/sh without touching the real launchd or systemd.
type scriptWorld struct {
	dir string
	log string
}

func newScriptWorld(t *testing.T, programs ...string) *scriptWorld {
	t.Helper()
	w := &scriptWorld{dir: t.TempDir()}
	w.log = filepath.Join(w.dir, "calls.log")
	for _, name := range programs {
		w.writeProgram(t, filepath.Join(w.dir, "bin", name), name)
	}
	return w
}

func (w *scriptWorld) writeProgram(t *testing.T, path, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho \""+name+" $*\" >> \"$CALLS\"\n"), 0o755))
}

func (w *scriptWorld) touch(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(w.dir, name)
	require.NoError(t, os.WriteFile(path, []byte("unit"), 0o644))
	return path
}

func (w *scriptWorld) run(t *testing.T, command []string) []string {
	t.Helper()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = []string{"PATH=" + filepath.Join(w.dir, "bin") + ":/usr/bin:/bin", "CALLS=" + w.log}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	calls, err := os.ReadFile(w.log)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(calls)), "\n")
}

func TestLaunchdScript(t *testing.T) {
	t.Run("wakes the scout while JobPilot is there", func(t *testing.T) {
		w := newScriptWorld(t, "launchctl")
		exe := filepath.Join(w.dir, "Job Pilot.app", "jobpilot")
		w.writeProgram(t, exe, "jobpilot")
		plist := w.touch(t, "agent.plist")

		calls := w.run(t, shellCommand(wakeOrRemove+launchdRemoveSelf, exe, scoutID, "gui/501/label", plist))

		assert.Equal(t, []string{"jobpilot scout wake " + scoutID}, calls)
		assert.FileExists(t, plist)
	})
	t.Run("removes the agent once JobPilot is gone", func(t *testing.T) {
		w := newScriptWorld(t, "launchctl")
		plist := w.touch(t, "agent.plist")

		calls := w.run(t, shellCommand(wakeOrRemove+launchdRemoveSelf,
			filepath.Join(w.dir, "gone", "jobpilot"), scoutID, "gui/501/label", plist))

		assert.Equal(t, []string{"launchctl bootout gui/501/label"}, calls)
		assert.NoFileExists(t, plist, "gone before the bootout that stops the script")
	})
}

func TestSystemdScript(t *testing.T) {
	t.Run("wakes the scout while JobPilot is there", func(t *testing.T) {
		w := newScriptWorld(t, "systemctl")
		exe := filepath.Join(w.dir, "usr", "bin", "jobpilot")
		w.writeProgram(t, exe, "jobpilot")
		timer, service := w.touch(t, "a.timer"), w.touch(t, "a.service")

		calls := w.run(t, shellCommand(wakeOrRemove+systemdRemoveSelf, exe, scoutID, "a.timer", timer, service))

		assert.Equal(t, []string{"jobpilot scout wake " + scoutID}, calls)
		assert.FileExists(t, timer)
		assert.FileExists(t, service)
	})
	t.Run("removes its units once JobPilot is gone", func(t *testing.T) {
		w := newScriptWorld(t, "systemctl")
		timer, service := w.touch(t, "a.timer"), w.touch(t, "a.service")

		calls := w.run(t, shellCommand(wakeOrRemove+systemdRemoveSelf,
			"/usr/bin/gone/jobpilot", scoutID, "a.timer", timer, service))

		assert.Equal(t, []string{"systemctl --user disable --now a.timer", "systemctl --user daemon-reload"}, calls)
		assert.NoFileExists(t, timer)
		assert.NoFileExists(t, service)
	})
}
