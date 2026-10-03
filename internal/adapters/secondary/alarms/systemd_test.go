package alarms

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

func newTestSystemd(t *testing.T, runner *fakeRunner) *Systemd {
	t.Helper()
	s := NewSystemd(t.TempDir())
	s.run = runner.run
	return s
}

func TestSystemdService(t *testing.T) {
	alarm := dailyAlarm("/opt/Job Pilot/100%/$HOME/jobpilot")
	alarm.Env = map[string]string{"XDG_CONFIG_HOME": "/home/ada/.cfg"}

	content, err := systemdService(alarm)
	require.NoError(t, err)
	unit := string(content)

	assert.Contains(t, unit, `ExecStart="/opt/Job Pilot/100%%/$$HOME/jobpilot" "scout" "wake" "`+scoutID+`"`,
		"a space is quoted, and % and $ are not read as a specifier or a variable")
	assert.Contains(t, unit, "Type=oneshot\n")
	assert.Contains(t, unit, "KillMode=process\n", "the daemon it starts outlives it")
	assert.Contains(t, unit, `Environment="XDG_CONFIG_HOME=/home/ada/.cfg"`)
	assert.Equal(t, alarm.Fingerprint(), readFingerprint(content))
}

func TestSystemdService_RefusesAnEnvironmentNameItCannotWrite(t *testing.T) {
	alarm := dailyAlarm("/usr/bin/jobpilot")
	alarm.Env = map[string]string{"A=B": "c"}
	_, err := systemdService(alarm)
	assert.Error(t, err)
}

func TestSystemdTimer(t *testing.T) {
	daily, err := systemdTimer(dailyAlarm("/usr/bin/jobpilot"))
	require.NoError(t, err)
	assert.Contains(t, string(daily), "OnCalendar=*-*-* 08:30:00\n")
	assert.Contains(t, string(daily), "Persistent=true\n", "a time missed while off runs once")
	assert.Contains(t, string(daily), "WantedBy=timers.target\n")

	hourly, err := systemdTimer(hourlyAlarm("/usr/bin/jobpilot"))
	require.NoError(t, err)
	assert.Contains(t, string(hourly), "OnCalendar=*-*-* *:00:00\n")
}

func TestSystemd_WriteListRemove(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{}
	s := newTestSystemd(t, runner)
	alarm := hourlyAlarm("/usr/bin/jobpilot")

	require.NoError(t, s.Write(ctx, alarm))

	timer := "jobpilot-scout-" + scoutID + ".timer"
	assert.Equal(t, []string{
		"systemctl --user show --property=Version",
		"systemctl --user daemon-reload",
		"systemctl --user enable " + timer,
		"systemctl --user restart " + timer,
	}, runner.calls)
	for _, suffix := range []string{".service", ".timer"} {
		_, err := os.Stat(s.path(scoutID, suffix))
		assert.NoError(t, err, suffix)
	}

	entries, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, alarm.Fingerprint(), entries[0].Fingerprint)
	assert.False(t, entries[0].SwitchedOff)

	require.NoError(t, s.Remove(ctx, scoutID))
	assert.Equal(t, 1, runner.ran("systemctl --user disable --now "+timer))
	for _, suffix := range []string{".service", ".timer"} {
		_, err := os.Stat(s.path(scoutID, suffix))
		assert.True(t, os.IsNotExist(err), suffix)
	}
}

func TestSystemd_ATimerTheUserDisabledStaysDisabled(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
		if strings.Contains(call, "is-enabled") {
			return []byte("disabled\n"), errFailed
		}
		return nil, nil
	}}
	s := newTestSystemd(t, runner)
	require.NoError(t, os.MkdirAll(s.dir, 0o755))
	require.NoError(t, os.WriteFile(s.path(scoutID, ".timer"), []byte("old"), 0o644))

	require.NoError(t, s.Write(ctx, dailyAlarm("/usr/bin/jobpilot")))

	assert.Zero(t, runner.ran("systemctl --user enable"))
	assert.Zero(t, runner.ran("systemctl --user restart"))
	entries, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].SwitchedOff)
}

func TestSystemd_WithoutAUserManagerSchedulingIsUnavailable(t *testing.T) {
	runner := &fakeRunner{respond: func(string, []string) ([]byte, error) {
		return []byte("Failed to connect to bus: No medium found"), errFailed
	}}
	s := newTestSystemd(t, runner)

	_, err := s.List(context.Background())
	assert.ErrorIs(t, err, ports.ErrAlarmsUnavailable)
	assert.ErrorIs(t, s.Write(context.Background(), dailyAlarm("/usr/bin/jobpilot")), ports.ErrAlarmsUnavailable)
}

func TestSystemdQuote(t *testing.T) {
	assert.Equal(t, `"plain"`, systemdQuote("plain"))
	assert.Equal(t, `"a \"b\" \\c"`, systemdQuote(`a "b" \c`))
	assert.Equal(t, `"50%% of $$PATH"`, systemdQuote("50% of $PATH"))
	assert.Equal(t, `"two\nlines"`, systemdQuote("two\nlines"))
}
