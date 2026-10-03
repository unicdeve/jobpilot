package alarms

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

const scoutID = "3f1c2a8e-5b7d-4e1a-9c3b-2d4e6f8a0b1c"

// fakeRunner records the programs an adapter runs and answers for them.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
	respond func(call string, args []string) ([]byte, error)
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	if f.respond == nil {
		return nil, nil
	}
	return f.respond(call, args)
}

func (f *fakeRunner) ran(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

func dailyAlarm(executable string) domain.ScoutAlarm {
	return domain.ScoutAlarm{
		ScoutID:    scoutID,
		Schedule:   domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 8*60 + 30},
		Executable: executable,
	}
}

func hourlyAlarm(executable string) domain.ScoutAlarm {
	return domain.ScoutAlarm{
		ScoutID:    scoutID,
		Schedule:   domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6},
		Executable: executable,
	}
}

func TestCheckScoutID(t *testing.T) {
	assert.NoError(t, checkScoutID(scoutID))
	for _, bad := range []string{"", "scout-1", "../../etc/passwd", scoutID + "\n<key>", `"; rm -rf ~`} {
		assert.Error(t, checkScoutID(bad), bad)
	}
}

func TestReadFingerprint(t *testing.T) {
	assert.Equal(t, "0a1b2c", readFingerprint([]byte("<!-- jobpilot-fingerprint: 0a1b2c -->")))
	assert.Equal(t, "", readFingerprint([]byte("written by someone else")))
}

var errFailed = errors.New("exit status 1")
