package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func scheduleCommand(t *testing.T, withOff bool, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "schedule"}
	addScheduleFlags(cmd)
	if withOff {
		cmd.Flags().Bool("off", false, "")
	}
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

func TestScheduleFromFlags(t *testing.T) {
	cases := []struct {
		name    string
		withOff bool
		args    []string
		want    domain.ScoutSchedule
		wantErr string
	}{
		{name: "nothing is off", want: domain.ScoutSchedule{Kind: domain.ScheduleOff}},
		{name: "every", args: []string{"--every", "6"}, want: domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6}},
		{name: "daily", args: []string{"--daily", "08:30"}, want: domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 510}},
		{name: "daily with a single-digit hour", args: []string{"--daily", "7:05"}, want: domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 425}},
		{name: "daily in words", args: []string{"--daily", "8am"}, wantErr: "HH:MM"},
		{name: "daily past midnight", args: []string{"--daily", "24:00"}, wantErr: "HH:MM"},
		{name: "off", withOff: true, args: []string{"--off"}, want: domain.ScoutSchedule{Kind: domain.ScheduleOff}},
		{name: "off and a time", withOff: true, args: []string{"--off", "--daily", "08:00"}, wantErr: "--off cannot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scheduleFromFlags(scheduleCommand(t, tc.withOff, tc.args...))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDescribeSchedule(t *testing.T) {
	assert.Equal(t, "only when you run it", describeSchedule(domain.ScoutSchedule{}))
	assert.Equal(t, "every hour", describeSchedule(domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 1}))
	assert.Equal(t, "every 6 hours", describeSchedule(domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6}))
	assert.Equal(t, "daily at 08:05", describeSchedule(domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 485}))
}
