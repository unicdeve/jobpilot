package alarms

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// launchdLabelPrefix names JobPilot's launchd agents. It is the app's
// bundle identifier, which is still the placeholder Tauri was created with;
// it should be settled before release, since changing it later means
// recognising both (ADR-0027 §7).
const launchdLabelPrefix = "com.user.jobpilot.scout."

// Launchd keeps scheduled Scouts as launchd agents in the user's
// LaunchAgents directory, one per Scout.
type Launchd struct {
	dir    string
	domain string
	run    runner
	// bootstrapRetries absorbs launchd removing an agent asynchronously:
	// bootstrapping a label straight after booting it out can fail until
	// the old one is gone.
	bootstrapRetries int
	retryDelay       time.Duration
}

// NewLaunchd returns the agents for the user whose home is home and whose
// user ID is uid.
func NewLaunchd(home string, uid int) *Launchd {
	return &Launchd{
		dir:              filepath.Join(home, "Library", "LaunchAgents"),
		domain:           fmt.Sprintf("gui/%d", uid),
		run:              runCommand,
		bootstrapRetries: 10,
		retryDelay:       200 * time.Millisecond,
	}
}

func (l *Launchd) label(scoutID string) string { return launchdLabelPrefix + scoutID }

func (l *Launchd) path(scoutID string) string { return filepath.Join(l.dir, l.label(scoutID)+".plist") }

// List returns the agents JobPilot wrote, and which the user switched off
// in System Settings → Login Items.
func (l *Launchd) List(ctx context.Context) ([]domain.ScoutAlarmEntry, error) {
	paths, err := filepath.Glob(filepath.Join(l.dir, launchdLabelPrefix+"*.plist"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	disabled, err := l.disabled(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]domain.ScoutAlarmEntry, 0, len(paths))
	for _, path := range paths {
		scoutID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), launchdLabelPrefix), ".plist")
		if checkScoutID(scoutID) != nil {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		entries = append(entries, domain.ScoutAlarmEntry{
			ScoutID:     scoutID,
			Fingerprint: readFingerprint(content),
			SwitchedOff: disabled[l.label(scoutID)],
		})
	}
	return entries, nil
}

// Write replaces the Scout's agent and loads it, unless the user switched
// it off, in which case the new file waits for them to switch it back on.
func (l *Launchd) Write(ctx context.Context, alarm domain.ScoutAlarm) error {
	if err := checkScoutID(alarm.ScoutID); err != nil {
		return err
	}
	content, err := launchdPlist(l.label(alarm.ScoutID), alarm)
	if err != nil {
		return err
	}
	disabled, err := l.disabled(ctx)
	if err != nil {
		return err
	}

	l.bootout(ctx, alarm.ScoutID)
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(l.path(alarm.ScoutID), content, 0o644); err != nil {
		return err
	}
	if disabled[l.label(alarm.ScoutID)] {
		return nil
	}

	for attempt := 0; ; attempt++ {
		_, err := l.run(ctx, "launchctl", "bootstrap", l.domain, l.path(alarm.ScoutID))
		if err == nil || attempt >= l.bootstrapRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(l.retryDelay):
		}
	}
}

// Remove unloads the Scout's agent and deletes its file.
func (l *Launchd) Remove(ctx context.Context, scoutID string) error {
	if err := checkScoutID(scoutID); err != nil {
		return err
	}
	l.bootout(ctx, scoutID)
	if err := os.Remove(l.path(scoutID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// bootout unloads an agent. One that is not loaded, because it was never
// bootstrapped or the user switched it off, is the same as one unloaded.
func (l *Launchd) bootout(ctx context.Context, scoutID string) {
	_, _ = l.run(ctx, "launchctl", "bootout", l.domain+"/"+l.label(scoutID))
}

// disabled reads which of JobPilot's labels launchd holds switched off.
//
// `launchctl print-disabled` prints one line per label it has an override
// for: `"label" => disabled` since macOS 13, `"label" => true` before.
func (l *Launchd) disabled(ctx context.Context) (map[string]bool, error) {
	out, err := l.run(ctx, "launchctl", "print-disabled", l.domain)
	if err != nil {
		return nil, err
	}
	disabled := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `"`+launchdLabelPrefix) {
			continue
		}
		label, state, ok := strings.Cut(line, "=>")
		if !ok {
			continue
		}
		state = strings.TrimSpace(state)
		disabled[strings.Trim(strings.TrimSpace(label), `"`)] = state == "disabled" || state == "true"
	}
	return disabled, nil
}

// launchdPlist is the agent for one Scout.
//
// RunAtLoad has it ask at login too, because launchd catches up a missed
// calendar interval after sleep but not after a shutdown (ADR-0027 §3);
// core decides whether that ask finds the Scout due.
//
// AbandonProcessGroup keeps the daemon the alarm starts alive after
// `scout wake` exits: launchd otherwise kills everything left in the job's
// process group.
//
// ProcessType Interactive, because the daemon an alarm starts is the one
// the desktop then talks to; under Background or the default, launchd
// would throttle it as it throttles a job nobody is waiting on.
func launchdPlist(label string, alarm domain.ScoutAlarm) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	fmt.Fprintf(&b, "<!-- Written by JobPilot to run a scout on its schedule. %s%s -->\n", fingerprintMarker, alarm.Fingerprint())
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")

	key := func(name string) { fmt.Fprintf(&b, "\t<key>%s</key>\n", name) }
	str := func(indent, value string) error {
		b.WriteString(indent)
		b.WriteString("<string>")
		if err := xml.EscapeText(&b, []byte(value)); err != nil {
			return err
		}
		b.WriteString("</string>\n")
		return nil
	}

	key("Label")
	if err := str("\t", label); err != nil {
		return nil, err
	}
	key("ProgramArguments")
	b.WriteString("\t<array>\n")
	for _, arg := range append([]string{alarm.Executable}, wakeArgs(alarm.ScoutID)...) {
		if err := str("\t\t", arg); err != nil {
			return nil, err
		}
	}
	b.WriteString("\t</array>\n")

	if len(alarm.Env) > 0 {
		key("EnvironmentVariables")
		b.WriteString("\t<dict>\n")
		names := make([]string, 0, len(alarm.Env))
		for name := range alarm.Env {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			b.WriteString("\t\t<key>")
			if err := xml.EscapeText(&b, []byte(name)); err != nil {
				return nil, err
			}
			b.WriteString("</key>\n")
			if err := str("\t\t", alarm.Env[name]); err != nil {
				return nil, err
			}
		}
		b.WriteString("\t</dict>\n")
	}

	key("StartCalendarInterval")
	b.WriteString("\t<dict>\n")
	switch alarm.Schedule.Kind {
	case domain.ScheduleDaily:
		fmt.Fprintf(&b, "\t\t<key>Hour</key>\n\t\t<integer>%d</integer>\n", alarm.Schedule.DailyAt/60)
		fmt.Fprintf(&b, "\t\t<key>Minute</key>\n\t\t<integer>%d</integer>\n", alarm.Schedule.DailyAt%60)
	case domain.ScheduleEvery:
		// Every hour, on the hour: core runs the Scout on the first hour
		// it is due (ADR-0027 §2).
		b.WriteString("\t\t<key>Minute</key>\n\t\t<integer>0</integer>\n")
	default:
		return nil, fmt.Errorf("a scout with schedule %q has no alarm", alarm.Schedule.Kind)
	}
	b.WriteString("\t</dict>\n")

	key("RunAtLoad")
	b.WriteString("\t<true/>\n")
	key("AbandonProcessGroup")
	b.WriteString("\t<true/>\n")
	key("ProcessType")
	b.WriteString("\t<string>Interactive</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes(), nil
}

// writeFileAtomic writes a file whole or not at all, so a scheduler never
// reads one half-written.
func writeFileAtomic(path string, content []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
