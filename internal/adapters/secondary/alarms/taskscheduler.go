package alarms

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// taskFolder is the Task Scheduler folder JobPilot's tasks live in, and
// taskPrefix the start of each task's full name.
const (
	taskFolder = `\JobPilot\`
	taskPrefix = taskFolder + "scout-"
)

// TaskScheduler keeps scheduled Scouts as Windows Task Scheduler tasks,
// one per Scout, created with schtasks from XML.
type TaskScheduler struct {
	run runner
	// tempDir is where a task's XML is written for schtasks to read.
	tempDir string
}

// NewTaskScheduler returns the tasks for the logged-in user.
func NewTaskScheduler() *TaskScheduler {
	return &TaskScheduler{run: runCommand, tempDir: os.TempDir()}
}

func (s *TaskScheduler) name(scoutID string) string { return taskPrefix + scoutID }

// List returns the tasks JobPilot created, and which the user disabled.
func (s *TaskScheduler) List(ctx context.Context) ([]domain.ScoutAlarmEntry, error) {
	out, err := s.run(ctx, "schtasks", "/Query", "/FO", "CSV", "/NH")
	if err != nil {
		return nil, err
	}
	names, err := taskNames(out)
	if err != nil {
		return nil, err
	}

	entries := make([]domain.ScoutAlarmEntry, 0, len(names))
	for _, name := range names {
		scoutID := strings.TrimPrefix(name, taskPrefix)
		if checkScoutID(scoutID) != nil {
			continue
		}
		definition, err := s.run(ctx, "schtasks", "/Query", "/TN", name, "/XML")
		if err != nil {
			return nil, err
		}
		definition = decodeConsoleText(definition)
		entries = append(entries, domain.ScoutAlarmEntry{
			ScoutID:     scoutID,
			Fingerprint: readFingerprint(definition),
			SwitchedOff: taskDisabled(definition),
		})
	}
	return entries, nil
}

// Write creates or replaces the Scout's task, keeping it disabled if the
// user disabled it.
func (s *TaskScheduler) Write(ctx context.Context, alarm domain.ScoutAlarm) error {
	if err := checkScoutID(alarm.ScoutID); err != nil {
		return err
	}
	enabled := true
	if existing, err := s.run(ctx, "schtasks", "/Query", "/TN", s.name(alarm.ScoutID), "/XML"); err == nil {
		enabled = !taskDisabled(decodeConsoleText(existing))
	}
	definition, err := taskXML(alarm, enabled)
	if err != nil {
		return err
	}

	file, err := os.CreateTemp(s.tempDir, "jobpilot-scout-*.xml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(utf16LE(definition)); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = s.run(ctx, "schtasks", "/Create", "/TN", s.name(alarm.ScoutID), "/XML", file.Name(), "/F")
	return err
}

// Remove deletes the Scout's task. A task that is not there is removed.
func (s *TaskScheduler) Remove(ctx context.Context, scoutID string) error {
	if err := checkScoutID(scoutID); err != nil {
		return err
	}
	_, err := s.run(ctx, "schtasks", "/Delete", "/TN", s.name(scoutID), "/F")
	if err == nil {
		return nil
	}
	// schtasks says a task is missing in the system's language, so whether
	// that is why it failed is asked rather than read from its words.
	if _, queryErr := s.run(ctx, "schtasks", "/Query", "/TN", s.name(scoutID)); queryErr != nil {
		return nil
	}
	return err
}

// taskNames reads JobPilot's tasks out of `schtasks /Query /FO CSV /NH`,
// whose first column is a task's full name. The names are not translated;
// the other columns are, and are not read.
func taskNames(out []byte) ([]string, error) {
	reader := csv.NewReader(bytes.NewReader(decodeConsoleText(out)))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading the scheduled tasks: %w", err)
	}
	seen := map[string]bool{}
	var names []string
	for _, record := range records {
		if len(record) == 0 || !strings.HasPrefix(record[0], taskPrefix) || seen[record[0]] {
			continue
		}
		seen[record[0]] = true
		names = append(names, record[0])
	}
	sort.Strings(names)
	return names, nil
}

var taskEnabledPattern = regexp.MustCompile(`(?s)<Settings>.*?<Enabled>\s*false\s*</Enabled>.*?</Settings>`)

// taskDisabled reports a task the user disabled, which Task Scheduler
// records as <Enabled>false</Enabled> in its settings.
func taskDisabled(definition []byte) bool {
	return taskEnabledPattern.Match(definition)
}

// taskXML is the task for one Scout.
//
// It runs as the logged-in user with no stored password, so it starts
// only while they are logged in and opens the browser on their desktop if
// a run is later prepared there. StartWhenAvailable runs a missed time
// once when the machine is next on (ADR-0027 §3), and the battery settings
// keep a laptop from skipping its runs.
//
// The daemon is a console program, so the task starts it through
// `conhost.exe --headless`, which gives it a console nobody sees instead
// of a window that flashes up on every alarm.
func taskXML(alarm domain.ScoutAlarm, enabled bool) ([]byte, error) {
	var trigger string
	switch alarm.Schedule.Kind {
	case domain.ScheduleDaily:
		trigger = fmt.Sprintf(`    <CalendarTrigger>
      <StartBoundary>2026-01-01T%02d:%02d:00</StartBoundary>
      <ScheduleByDay>
        <DaysInterval>1</DaysInterval>
      </ScheduleByDay>
    </CalendarTrigger>
`, alarm.Schedule.DailyAt/60, alarm.Schedule.DailyAt%60)
	case domain.ScheduleEvery:
		// Every hour, on the hour: core runs the Scout on the first hour
		// it is due (ADR-0027 §2).
		trigger = `    <CalendarTrigger>
      <StartBoundary>2026-01-01T00:00:00</StartBoundary>
      <Repetition>
        <Interval>PT1H</Interval>
        <Duration>P1D</Duration>
      </Repetition>
      <ScheduleByDay>
        <DaysInterval>1</DaysInterval>
      </ScheduleByDay>
    </CalendarTrigger>
`
	default:
		return nil, fmt.Errorf("a scout with schedule %q has no alarm", alarm.Schedule.Kind)
	}

	escape := func(value string) (string, error) {
		var b bytes.Buffer
		err := xml.EscapeText(&b, []byte(value))
		return b.String(), err
	}
	arguments := []string{"--headless", windowsQuote(alarm.Executable)}
	for _, arg := range wakeArgs(alarm.ScoutID) {
		arguments = append(arguments, windowsQuote(arg))
	}
	args, err := escape(strings.Join(arguments, " "))
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
`)
	fmt.Fprintf(&b, "    <Description>Runs a JobPilot scout on its schedule. %s%s</Description>\n",
		fingerprintMarker, alarm.Fingerprint())
	b.WriteString(`  </RegistrationInfo>
  <Triggers>
`)
	b.WriteString(trigger)
	b.WriteString(`  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT5M</ExecutionTimeLimit>
`)
	fmt.Fprintf(&b, "    <Enabled>%t</Enabled>\n", enabled)
	b.WriteString(`  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>conhost.exe</Command>
`)
	fmt.Fprintf(&b, "      <Arguments>%s</Arguments>\n", args)
	b.WriteString(`    </Exec>
  </Actions>
</Task>
`)
	return []byte(b.String()), nil
}

// windowsQuote quotes one argument as a Windows program reads its command
// line: in double quotes, with a quote escaped and the backslashes before
// a quote, or before the closing quote, doubled.
func windowsQuote(arg string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			slashes++
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, 2*slashes+1))
		default:
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, 2*slashes))
	b.WriteByte('"')
	return b.String()
}

// utf16LE encodes text as UTF-16 little-endian with a byte-order mark,
// which is what schtasks reads task XML as.
func utf16LE(text []byte) []byte {
	units := utf16.Encode([]rune(string(text)))
	out := make([]byte, 2+2*len(units))
	out[0], out[1] = 0xFF, 0xFE
	for i, unit := range units {
		binary.LittleEndian.PutUint16(out[2+2*i:], unit)
	}
	return out
}

// decodeConsoleText reads what schtasks printed, which is UTF-16 when it
// starts with a byte-order mark and single-byte text otherwise.
func decodeConsoleText(out []byte) []byte {
	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xFE {
		return out
	}
	body := out[2:]
	units := make([]uint16, len(body)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(body[2*i:])
	}
	return []byte(string(utf16.Decode(units)))
}
