package alarms

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newTestLaunchd(t *testing.T, runner *fakeRunner) *Launchd {
	t.Helper()
	l := NewLaunchd(t.TempDir(), 501)
	l.run = runner.run
	l.retryDelay = 0
	return l
}

func TestLaunchdPlist(t *testing.T) {
	alarm := dailyAlarm("/Applications/Jobs & Me.app/Contents/MacOS/jobpilot")
	alarm.Env = map[string]string{"HOME": "/Users/ada"}

	label := launchdLabelPrefix + scoutID
	content, err := launchdPlist(label, "/Users/ada/Library/LaunchAgents/"+label+".plist", "gui/501", alarm)
	require.NoError(t, err)
	plist := string(content)

	assert.Contains(t, plist, "<string>io.github.unicdeve.jobpilot.scout."+scoutID+"</string>")
	assert.Contains(t, plist, "<string>/bin/sh</string>\n\t\t<string>-c</string>\n\t\t<string>"+
		`if [ -x &#34;$1&#34; ]; then exec &#34;$1&#34; scout wake &#34;$2&#34;; fi; rm -f &#34;$4&#34;; exec launchctl bootout &#34;$3&#34;`+
		"</string>\n\t\t<string>jobpilot-scout</string>\n"+
		"\t\t<string>/Applications/Jobs &amp; Me.app/Contents/MacOS/jobpilot</string>\n"+
		"\t\t<string>"+scoutID+"</string>\n"+
		"\t\t<string>gui/501/"+label+"</string>\n"+
		"\t\t<string>/Users/ada/Library/LaunchAgents/"+label+".plist</string>\n",
		"a script that wakes the scout or removes the agent, with every path an argument and escaped")
	assert.Contains(t, plist, "<key>AssociatedBundleIdentifiers</key>\n\t<array>\n\t\t<string>io.github.unicdeve.jobpilot</string>\n\t</array>",
		"Login Items shows it as JobPilot's")
	assert.Contains(t, plist, "<key>Hour</key>\n\t\t<integer>8</integer>\n\t\t<key>Minute</key>\n\t\t<integer>30</integer>")
	assert.Contains(t, plist, "<key>RunAtLoad</key>\n\t<true/>", "it asks at login, which a shutdown would otherwise lose")
	assert.Contains(t, plist, "<key>AbandonProcessGroup</key>\n\t<true/>", "the daemon it starts outlives it")
	assert.Contains(t, plist, "<key>HOME</key>\n\t\t<string>/Users/ada</string>")
	assert.Equal(t, alarm.Fingerprint(), readFingerprint(content))

	lintPlist(t, content)
}

func TestLaunchdPlist_EveryHours(t *testing.T) {
	content, err := launchdPlist(launchdLabelPrefix+scoutID, "/tmp/agent.plist", "gui/501", hourlyAlarm("/usr/local/bin/jobpilot"))
	require.NoError(t, err)

	assert.Contains(t, string(content), "<key>StartCalendarInterval</key>\n\t<dict>\n\t\t<key>Minute</key>\n\t\t<integer>0</integer>\n\t</dict>",
		"hourly, on the hour; core decides which hour finds it due")
	assert.NotContains(t, string(content), "<key>Hour</key>")
	lintPlist(t, content)
}

// lintPlist has macOS check the plist, where there is a macOS to ask.
func lintPlist(t *testing.T, content []byte) {
	t.Helper()
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		return
	}
	path := filepath.Join(t.TempDir(), "agent.plist")
	require.NoError(t, os.WriteFile(path, content, 0o644))
	out, err := exec.Command(plutil, "-lint", path).CombinedOutput()
	assert.NoError(t, err, string(out))
}

func TestLaunchd_WriteListRemove(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{}
	l := newTestLaunchd(t, runner)
	alarm := dailyAlarm("/usr/local/bin/jobpilot")

	require.NoError(t, l.Write(ctx, alarm))

	assert.Equal(t, []string{
		"launchctl print-disabled gui/501",
		"launchctl bootout gui/501/io.github.unicdeve.jobpilot.scout." + scoutID,
		"launchctl bootstrap gui/501 " + l.path(scoutID),
	}, runner.calls)
	info, err := os.Stat(l.path(scoutID))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	entries, err := l.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, domain.ScoutAlarmEntry{ScoutID: scoutID, Fingerprint: alarm.Fingerprint()}, entries[0])

	require.NoError(t, l.Remove(ctx, scoutID))
	_, err = os.Stat(l.path(scoutID))
	assert.True(t, os.IsNotExist(err))
	require.NoError(t, l.Remove(ctx, scoutID), "removing what is not there is not an error")
}

func TestLaunchd_ListIgnoresWhatIsNotJobPilots(t *testing.T) {
	l := newTestLaunchd(t, &fakeRunner{})
	require.NoError(t, os.MkdirAll(l.dir, 0o755))
	for _, name := range []string{"com.apple.something.plist", launchdLabelPrefix + "not-a-uuid.plist"} {
		require.NoError(t, os.WriteFile(filepath.Join(l.dir, name), []byte("x"), 0o644))
	}

	entries, err := l.List(context.Background())

	require.NoError(t, err)
	assert.Empty(t, entries)
}

// writeLegacyAgent leaves an agent under the prefix used before the
// identifier was settled, as a daemon built before then wrote it.
func writeLegacyAgent(t *testing.T, l *Launchd) {
	t.Helper()
	content, err := launchdPlist(l.legacyLabel(scoutID), l.legacyPath(scoutID), l.domain, dailyAlarm("/usr/local/bin/jobpilot"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(l.dir, 0o755))
	require.NoError(t, os.WriteFile(l.legacyPath(scoutID), content, 0o644))
}

func TestLaunchd_ALegacyAgentIsListedToBeRewritten(t *testing.T) {
	l := newTestLaunchd(t, &fakeRunner{})
	writeLegacyAgent(t, l)

	entries, err := l.List(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []domain.ScoutAlarmEntry{{ScoutID: scoutID}}, entries,
		"no fingerprint, so a reconcile writes it again under the current prefix")
}

func TestLaunchd_ALegacyAgentBesideACurrentOneIsListedOnce(t *testing.T) {
	ctx := context.Background()
	l := newTestLaunchd(t, &fakeRunner{})
	require.NoError(t, l.Write(ctx, dailyAlarm("/usr/local/bin/jobpilot")))
	writeLegacyAgent(t, l)

	entries, err := l.List(ctx)

	require.NoError(t, err)
	assert.Equal(t, []domain.ScoutAlarmEntry{{ScoutID: scoutID}}, entries,
		"rewritten, so the legacy agent is removed")
}

func TestLaunchd_WriteMovesALegacyAgent(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{}
	l := newTestLaunchd(t, runner)
	writeLegacyAgent(t, l)

	require.NoError(t, l.Write(ctx, dailyAlarm("/usr/local/bin/jobpilot")))

	assert.Equal(t, []string{
		"launchctl print-disabled gui/501",
		"launchctl bootout gui/501/com.user.jobpilot.scout." + scoutID,
		"launchctl bootout gui/501/io.github.unicdeve.jobpilot.scout." + scoutID,
		"launchctl bootstrap gui/501 " + l.path(scoutID),
	}, runner.calls)
	_, err := os.Stat(l.legacyPath(scoutID))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(l.path(scoutID))
	assert.NoError(t, err)
}

func TestLaunchd_ALegacyAgentSwitchedOffStaysOffWhenMoved(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
		if strings.HasPrefix(call, "launchctl print-disabled") {
			return []byte("disabled services = {\n\t\"" + legacyLaunchdLabelPrefix + scoutID + "\" => disabled\n}\n"), nil
		}
		return nil, nil
	}}
	l := newTestLaunchd(t, runner)
	writeLegacyAgent(t, l)

	entries, err := l.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].SwitchedOff)

	require.NoError(t, l.Write(ctx, dailyAlarm("/usr/local/bin/jobpilot")))

	assert.Zero(t, runner.ran("launchctl bootstrap"), "loading it would turn it back on behind the user's back")
	assert.Equal(t, 1, runner.ran("launchctl disable gui/501/"+launchdLabelPrefix+scoutID),
		"launchd holds the switch against the old label, so it is said again for the new one")
}

func TestLaunchd_RemoveTakesALegacyAgentToo(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{}
	l := newTestLaunchd(t, runner)
	writeLegacyAgent(t, l)

	require.NoError(t, l.Remove(ctx, scoutID))

	_, err := os.Stat(l.legacyPath(scoutID))
	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, 1, runner.ran("launchctl bootout gui/501/"+legacyLaunchdLabelPrefix+scoutID))
	entries, err := l.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLaunchd_AnAgentSwitchedOffStaysOff(t *testing.T) {
	for name, state := range map[string]string{"macOS 13 and later": "disabled", "before macOS 13": "true"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
				if strings.HasPrefix(call, "launchctl print-disabled") {
					return []byte("disabled services = {\n\t\"com.apple.Siri.agent\" => disabled\n\t\"" +
						launchdLabelPrefix + scoutID + "\" => " + state + "\n}\n"), nil
				}
				return nil, nil
			}}
			l := newTestLaunchd(t, runner)

			require.NoError(t, l.Write(ctx, dailyAlarm("/usr/local/bin/jobpilot")))

			assert.Zero(t, runner.ran("launchctl bootstrap"), "loading it would turn it back on behind the user's back")
			entries, err := l.List(ctx)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.True(t, entries[0].SwitchedOff)
		})
	}
}

func TestLaunchd_BootstrapIsRetriedWhileTheOldAgentGoes(t *testing.T) {
	failures := 2
	runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
		if strings.HasPrefix(call, "launchctl bootstrap") && failures > 0 {
			failures--
			return nil, errFailed
		}
		return nil, nil
	}}
	l := newTestLaunchd(t, runner)

	require.NoError(t, l.Write(context.Background(), dailyAlarm("/usr/local/bin/jobpilot")))
	assert.Equal(t, 3, runner.ran("launchctl bootstrap"))
}

func TestLaunchd_ABootstrapThatKeepsFailingIsReported(t *testing.T) {
	runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
		if strings.HasPrefix(call, "launchctl bootstrap") {
			return nil, errFailed
		}
		return nil, nil
	}}
	l := newTestLaunchd(t, runner)

	assert.ErrorIs(t, l.Write(context.Background(), dailyAlarm("/usr/local/bin/jobpilot")), errFailed)
}

func TestLaunchd_RefusesAnIDThatIsNotAUUID(t *testing.T) {
	runner := &fakeRunner{}
	l := newTestLaunchd(t, runner)
	alarm := dailyAlarm("/usr/local/bin/jobpilot")
	alarm.ScoutID = "../../../Library/LaunchDaemons/evil"

	assert.Error(t, l.Write(context.Background(), alarm))
	assert.Error(t, l.Remove(context.Background(), alarm.ScoutID))
	assert.Empty(t, runner.calls)
}
