package alarms

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// systemdUnitPrefix names JobPilot's systemd user units.
const systemdUnitPrefix = "jobpilot-scout-"

// Systemd keeps scheduled Scouts as systemd user timers, each with the
// one-shot service it starts.
type Systemd struct {
	dir string
	run runner
}

// NewSystemd returns the timers kept in configDir/systemd/user, where
// configDir is the user's configuration directory ($XDG_CONFIG_HOME, or
// ~/.config), which is where systemd looks for a user's own units.
func NewSystemd(configDir string) *Systemd {
	return &Systemd{dir: filepath.Join(configDir, "systemd", "user"), run: runCommand}
}

func (s *Systemd) unit(scoutID, suffix string) string { return systemdUnitPrefix + scoutID + suffix }

func (s *Systemd) path(scoutID, suffix string) string {
	return filepath.Join(s.dir, s.unit(scoutID, suffix))
}

func (s *Systemd) systemctl(ctx context.Context, args ...string) ([]byte, error) {
	return s.run(ctx, "systemctl", append([]string{"--user"}, args...)...)
}

// available reports whether there is a systemd user manager to talk to.
// A system without systemd, or a session without a user manager, has none,
// and scheduling is unavailable there rather than broken.
func (s *Systemd) available(ctx context.Context) error {
	if _, err := s.systemctl(ctx, "show", "--property=Version"); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrAlarmsUnavailable, err)
	}
	return nil
}

// List returns the timers JobPilot wrote, and which are disabled.
func (s *Systemd) List(ctx context.Context) ([]domain.ScoutAlarmEntry, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(s.dir, systemdUnitPrefix+"*.timer"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	entries := make([]domain.ScoutAlarmEntry, 0, len(paths))
	for _, path := range paths {
		scoutID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), systemdUnitPrefix), ".timer")
		if checkScoutID(scoutID) != nil {
			continue
		}
		content, err := os.ReadFile(s.path(scoutID, ".service"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		entries = append(entries, domain.ScoutAlarmEntry{
			ScoutID:     scoutID,
			Fingerprint: readFingerprint(content),
			SwitchedOff: s.isDisabled(ctx, scoutID),
		})
	}
	return entries, nil
}

// isDisabled reports a timer the user disabled with systemctl.
// `systemctl is-enabled` exits non-zero for a disabled unit and prints
// "disabled", so the answer is in what it prints, not in its error.
func (s *Systemd) isDisabled(ctx context.Context, scoutID string) bool {
	out, _ := s.systemctl(ctx, "is-enabled", s.unit(scoutID, ".timer"))
	return strings.TrimSpace(string(out)) == "disabled"
}

// Write replaces the Scout's service and timer, and enables the timer,
// unless the user disabled it, in which case the new files wait for them
// to enable it again.
func (s *Systemd) Write(ctx context.Context, alarm domain.ScoutAlarm) error {
	if err := checkScoutID(alarm.ScoutID); err != nil {
		return err
	}
	service, err := systemdService(alarm, s.path(alarm.ScoutID, ".timer"), s.path(alarm.ScoutID, ".service"))
	if err != nil {
		return err
	}
	timer, err := systemdTimer(alarm)
	if err != nil {
		return err
	}
	if err := s.available(ctx); err != nil {
		return err
	}

	_, existed := os.Stat(s.path(alarm.ScoutID, ".timer"))
	keepOff := existed == nil && s.isDisabled(ctx, alarm.ScoutID)

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(s.path(alarm.ScoutID, ".service"), service, 0o644); err != nil {
		return err
	}
	if err := writeFileAtomic(s.path(alarm.ScoutID, ".timer"), timer, 0o644); err != nil {
		return err
	}
	if _, err := s.systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if keepOff {
		return nil
	}
	if _, err := s.systemctl(ctx, "enable", s.unit(alarm.ScoutID, ".timer")); err != nil {
		return err
	}
	// Restarted rather than started, so a changed time takes effect.
	_, err = s.systemctl(ctx, "restart", s.unit(alarm.ScoutID, ".timer"))
	return err
}

// Remove stops and disables the Scout's timer and deletes both units.
func (s *Systemd) Remove(ctx context.Context, scoutID string) error {
	if err := checkScoutID(scoutID); err != nil {
		return err
	}
	// A timer that was never enabled, or is already gone, is as good as
	// disabled.
	_, _ = s.systemctl(ctx, "disable", "--now", s.unit(scoutID, ".timer"))
	for _, suffix := range []string{".timer", ".service"} {
		if err := os.Remove(s.path(scoutID, suffix)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	_, err := s.systemctl(ctx, "daemon-reload")
	return err
}

// systemdRemoveSelf is the rest of a service's script once its executable
// is gone: $3 is the timer, and $4 and $5 the timer's and the service's
// files.
const systemdRemoveSelf = `systemctl --user disable --now "$3"; rm -f "$4" "$5"; exec systemctl --user daemon-reload`

// systemdService is the one-shot service a Scout's timer starts, whose
// timer and own unit are at timerPath and servicePath. It runs a script
// that wakes the Scout, or removes both units once JobPilot is gone
// (wakeOrRemove).
//
// KillMode=process keeps the daemon `scout wake` starts alive after it
// exits: systemd otherwise stops everything left in the service's cgroup
// when its main process ends.
func systemdService(alarm domain.ScoutAlarm, timerPath, servicePath string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# Written by JobPilot to run a scout on its schedule.\n")
	fmt.Fprintf(&b, "# %s%s\n", fingerprintMarker, alarm.Fingerprint())
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=JobPilot scout %s\n", alarm.ScoutID)
	b.WriteString("\n[Service]\nType=oneshot\n")

	timer := systemdUnitPrefix + alarm.ScoutID + ".timer"
	var command []string
	for _, arg := range shellCommand(wakeOrRemove+systemdRemoveSelf,
		alarm.Executable, alarm.ScoutID, timer, timerPath, servicePath) {
		command = append(command, systemdQuote(arg))
	}
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(command, " "))

	names := make([]string, 0, len(alarm.Env))
	for name := range alarm.Env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.ContainsAny(name, "= \t\n\"\\") || name == "" {
			return nil, fmt.Errorf("cannot write environment variable %q into a unit", name)
		}
		fmt.Fprintf(&b, "Environment=%s\n", systemdQuote(name+"="+alarm.Env[name]))
	}
	b.WriteString("KillMode=process\n")
	return []byte(b.String()), nil
}

// systemdTimer is the timer for a Scout's schedule.
//
// Persistent=true runs a time that passed while the machine was off or
// asleep once, when it is next on (ADR-0027 §3).
func systemdTimer(alarm domain.ScoutAlarm) ([]byte, error) {
	var calendar string
	switch alarm.Schedule.Kind {
	case domain.ScheduleDaily:
		calendar = fmt.Sprintf("*-*-* %02d:%02d:00", alarm.Schedule.DailyAt/60, alarm.Schedule.DailyAt%60)
	case domain.ScheduleEvery:
		// Every hour, on the hour: core runs the Scout on the first hour
		// it is due (ADR-0027 §2).
		calendar = "*-*-* *:00:00"
	default:
		return nil, fmt.Errorf("a scout with schedule %q has no alarm", alarm.Schedule.Kind)
	}

	var b strings.Builder
	b.WriteString("# Written by JobPilot to run a scout on its schedule.\n")
	fmt.Fprintf(&b, "# %s%s\n", fingerprintMarker, alarm.Fingerprint())
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=JobPilot scout %s, on its schedule\n", alarm.ScoutID)
	b.WriteString("\n[Timer]\n")
	fmt.Fprintf(&b, "OnCalendar=%s\n", calendar)
	b.WriteString("Persistent=true\n")
	b.WriteString("\n[Install]\nWantedBy=timers.target\n")
	return []byte(b.String()), nil
}

// systemdQuote quotes one word of a unit's command line or environment.
// Inside double quotes systemd still reads backslash escapes, expands "%"
// as a specifier and "$" as a variable, so each is escaped, and a newline,
// which would end the line, is written as its escape.
func systemdQuote(value string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$", "\n", `\n`)
	return `"` + r.Replace(value) + `"`
}
