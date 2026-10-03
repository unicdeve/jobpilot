package alarms

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// task is the part of a task definition the tests read back.
type task struct {
	Description string `xml:"RegistrationInfo>Description"`
	Calendar    struct {
		StartBoundary string `xml:"StartBoundary"`
		Interval      string `xml:"Repetition>Interval"`
		Duration      string `xml:"Repetition>Duration"`
		DaysInterval  int    `xml:"ScheduleByDay>DaysInterval"`
	} `xml:"Triggers>CalendarTrigger"`
	LogonType string `xml:"Principals>Principal>LogonType"`
	Settings  struct {
		DisallowStartIfOnBatteries bool `xml:"DisallowStartIfOnBatteries"`
		StopIfGoingOnBatteries     bool `xml:"StopIfGoingOnBatteries"`
		StartWhenAvailable         bool `xml:"StartWhenAvailable"`
		Enabled                    bool `xml:"Enabled"`
	} `xml:"Settings"`
	Command   string `xml:"Actions>Exec>Command"`
	Arguments string `xml:"Actions>Exec>Arguments"`
}

func parseTask(t *testing.T, definition []byte) task {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(definition))
	// The declaration says UTF-16, which the file schtasks reads is; the
	// text in hand is already decoded.
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	var parsed task
	require.NoError(t, decoder.Decode(&parsed))
	return parsed
}

func TestTaskXML_Daily(t *testing.T) {
	alarm := dailyAlarm(`C:\Program Files\JobPilot & Co\jobpilot.exe`)

	definition, err := taskXML(alarm, true)
	require.NoError(t, err)
	parsed := parseTask(t, definition)

	assert.Equal(t, "2026-01-01T08:30:00", parsed.Calendar.StartBoundary)
	assert.Equal(t, 1, parsed.Calendar.DaysInterval)
	assert.Empty(t, parsed.Calendar.Interval)
	assert.Equal(t, "InteractiveToken", parsed.LogonType, "it runs as the logged-in user, with no password stored")
	assert.False(t, parsed.Settings.DisallowStartIfOnBatteries, "a laptop on battery still runs it")
	assert.False(t, parsed.Settings.StopIfGoingOnBatteries)
	assert.True(t, parsed.Settings.StartWhenAvailable, "a time missed while off runs once")
	assert.True(t, parsed.Settings.Enabled)
	assert.Equal(t, "conhost.exe", parsed.Command)
	assert.Equal(t, `--headless "C:\Program Files\JobPilot & Co\jobpilot.exe" "scout" "wake" "`+scoutID+`"`, parsed.Arguments)
	assert.Equal(t, alarm.Fingerprint(), readFingerprint([]byte(parsed.Description)))
}

func TestTaskXML_EveryHours(t *testing.T) {
	definition, err := taskXML(hourlyAlarm(`C:\jobpilot.exe`), false)
	require.NoError(t, err)
	parsed := parseTask(t, definition)

	assert.Equal(t, "2026-01-01T00:00:00", parsed.Calendar.StartBoundary)
	assert.Equal(t, "PT1H", parsed.Calendar.Interval, "hourly, on the hour; core decides which hour finds it due")
	assert.Equal(t, "P1D", parsed.Calendar.Duration)
	assert.False(t, parsed.Settings.Enabled)
}

func TestWindowsQuote(t *testing.T) {
	cases := map[string]string{
		`C:\Program Files\jobpilot.exe`: `"C:\Program Files\jobpilot.exe"`,
		`C:\dir\`:                       `"C:\dir\\"`,
		`say "hi"`:                      `"say \"hi\""`,
		`a\"b`:                          `"a\\\"b"`,
		``:                              `""`,
	}
	for in, want := range cases {
		assert.Equal(t, want, windowsQuote(in), in)
	}
}

func TestUTF16RoundTrip(t *testing.T) {
	text := []byte("<Task>Ünïcödé — scout</Task>")
	encoded := utf16LE(text)
	assert.Equal(t, []byte{0xFF, 0xFE}, encoded[:2])
	assert.Equal(t, text, decodeConsoleText(encoded))
	assert.Equal(t, text, decodeConsoleText(text), "text without a byte-order mark is left as it is")
}

func TestTaskNames(t *testing.T) {
	out := []byte(`"\Microsoft\Windows\Defrag\ScheduledDefrag","N/D","Prêt"
"\JobPilot\scout-` + scoutID + `","03/10/2026 08:30:00","Prêt"
"\JobPilot\scout-` + scoutID + `","03/10/2026 09:30:00","Prêt"
"\JobPilot\other","N/D","Désactivé"
`)
	names, err := taskNames(out)
	require.NoError(t, err)
	assert.Equal(t, []string{`\JobPilot\scout-` + scoutID}, names, "listed once, whatever language the rest is in")
}

func TestTaskScheduler_Write(t *testing.T) {
	ctx := context.Background()
	var written []byte
	runner := &fakeRunner{respond: func(call string, args []string) ([]byte, error) {
		switch {
		case strings.HasPrefix(call, "schtasks /Query"):
			return nil, errFailed
		case strings.HasPrefix(call, "schtasks /Create"):
			content, err := os.ReadFile(args[4])
			require.NoError(t, err)
			written = content
		}
		return nil, nil
	}}
	s := NewTaskScheduler()
	s.run, s.tempDir = runner.run, t.TempDir()

	require.NoError(t, s.Write(ctx, dailyAlarm(`C:\jobpilot.exe`)))

	assert.Equal(t, 1, runner.ran(`schtasks /Create /TN \JobPilot\scout-`+scoutID+" /XML "))
	require.NotEmpty(t, written)
	assert.Equal(t, []byte{0xFF, 0xFE}, written[:2], "schtasks reads task XML as UTF-16")
	assert.True(t, parseTask(t, decodeConsoleText(written)).Settings.Enabled)
	entries, err := os.ReadDir(s.tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the XML handed to schtasks is not left behind")
}

func TestTaskScheduler_ATaskTheUserDisabledStaysDisabled(t *testing.T) {
	ctx := context.Background()
	existing, err := taskXML(dailyAlarm(`C:\old\jobpilot.exe`), false)
	require.NoError(t, err)
	var written []byte
	runner := &fakeRunner{respond: func(call string, args []string) ([]byte, error) {
		switch {
		case strings.HasPrefix(call, "schtasks /Query /FO CSV"):
			return []byte(`"\JobPilot\scout-` + scoutID + `","N/A","Disabled"` + "\n"), nil
		case strings.HasPrefix(call, "schtasks /Query /TN"):
			return utf16LE(existing), nil
		case strings.HasPrefix(call, "schtasks /Create"):
			content, err := os.ReadFile(args[4])
			require.NoError(t, err)
			written = content
		}
		return nil, nil
	}}
	s := NewTaskScheduler()
	s.run, s.tempDir = runner.run, t.TempDir()

	require.NoError(t, s.Write(ctx, dailyAlarm(`C:\new\jobpilot.exe`)))
	assert.False(t, parseTask(t, decodeConsoleText(written)).Settings.Enabled)

	entries, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].SwitchedOff)
	assert.Equal(t, dailyAlarm(`C:\old\jobpilot.exe`).Fingerprint(), entries[0].Fingerprint)
}

func TestTaskScheduler_Remove(t *testing.T) {
	ctx := context.Background()

	t.Run("a task that is not there is removed", func(t *testing.T) {
		runner := &fakeRunner{respond: func(string, []string) ([]byte, error) { return nil, errFailed }}
		s := NewTaskScheduler()
		s.run = runner.run
		assert.NoError(t, s.Remove(ctx, scoutID))
	})

	t.Run("a task that is there and cannot be deleted is an error", func(t *testing.T) {
		runner := &fakeRunner{respond: func(call string, _ []string) ([]byte, error) {
			if strings.HasPrefix(call, "schtasks /Delete") {
				return nil, errFailed
			}
			return nil, nil
		}}
		s := NewTaskScheduler()
		s.run = runner.run
		assert.ErrorIs(t, s.Remove(ctx, scoutID), errFailed)
	})
}
