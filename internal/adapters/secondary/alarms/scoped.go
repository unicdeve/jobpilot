package alarms

import (
	"context"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

type scoped struct {
	base   ports.ScoutAlarms
	prefix string
	legacy bool
}

// Scoped restricts alarm listing and removal to one workspace and gives its
// operating-system entries collision-free names.
func Scoped(base ports.ScoutAlarms, workspaceID string) ports.ScoutAlarms {
	return &scoped{base: base, prefix: workspaceID + "."}
}

// LegacyScoped restricts alarm listing to the original workspace's
// unqualified entries. Its writes remain unqualified for compatibility with
// alarms created before local workspaces existed.
func LegacyScoped(base ports.ScoutAlarms) ports.ScoutAlarms {
	return &scoped{base: base, legacy: true}
}

func (s *scoped) List(ctx context.Context) ([]domain.ScoutAlarmEntry, error) {
	entries, err := s.base.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ScoutAlarmEntry, 0, len(entries))
	for _, entry := range entries {
		if s.legacy {
			if !strings.Contains(entry.ScoutID, ".") {
				result = append(result, entry)
			}
			continue
		}
		if !strings.HasPrefix(entry.ScoutID, s.prefix) {
			continue
		}
		entry.ScoutID = strings.TrimPrefix(entry.ScoutID, s.prefix)
		result = append(result, entry)
	}
	return result, nil
}

func (s *scoped) Write(ctx context.Context, alarm domain.ScoutAlarm) error {
	if !s.legacy {
		alarm.ScoutID = s.prefix + alarm.ScoutID
	}
	return s.base.Write(ctx, alarm)
}

func (s *scoped) Remove(ctx context.Context, scoutID string) error {
	if s.legacy {
		return s.base.Remove(ctx, scoutID)
	}
	return s.base.Remove(ctx, s.prefix+scoutID)
}
