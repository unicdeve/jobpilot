package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The sentences are from postings measured on 2026-10-01
// (docs/02_features/18_time_zones.md).
func TestReadHours_Requirements(t *testing.T) {
	t.Parallel()

	for sentence, want := range map[string][2]float64{
		// Ranges.
		"To create the best experience for our new hire, this role requires you to be based within UTC−6 to UTC+3.": {-6, 3},
		"To create the best experience for our new hire, this role requires you to be based within +3 / -7 UTC.":    {-7, 3},
		"To create the best experience for our new hire, this role requires you to be based within UTC -4 / -7 .":   {-7, -4},
		"as long as your main location is between UTC-4 and UTC+4.":                                                 {-4, 4},
		"UTC +3 to UTC -5": {-5, 3},
		// A zone and a margin.
		"Located in CET timezone (+/- 3 hours), we are unable to consider applications from candidates in other time zones.": {-2, 4},
		"Time zone: CET (+/- 3 hours).": {-2, 4},
		// Overlap with a zone's working day.
		"Able to overlap at least 4 hours a day with US Eastern Time":                                   {-9, -1},
		"Overlap: 6hs with East Coast time zone, USA":                                                   {-7, -3},
		"At least four hours of weekday overlap with Bangkok (GMT+7)":                                   {3, 11},
		"Able to provide at least 7 hours of overlap with the 9 am-5 pm EST workday (Monday - Friday).": {-6, -4},
		"To create the best experience for our new hire, this role requires at least 3-4 hours of overlap with Central European Time (CET/CEST) business hours to support collaboration with the team.": {-4, 6},
		// Hours of the clock.
		"Mandatory availability from 9:00 AM – 12:00 PM PST for daily team syncs and real-time collaboration.": {-8, -3},
		"Our team meetings are scheduled between 11:00 and 16:00 UTC.":                                         {-2, 1},
		// A zone's working hours.
		"This position requires working hours in CEST.":                            {1, 1},
		"in LATAM working EST hours.":                                              {-5, -5},
		"Full-time availability during EST or PST working hours.":                  {-8, -5},
		"The ability to operate in ET, CT, MT, or PT timezones is required.":       {-8, -5},
		"Must be located in East or Central timezones in the US.":                  {-6, -5},
		"100% Remote but ONLY residents of countries with US time zones (EST-PST)": {-8, -5},
		// Regions as time zones.
		"These roles are home based in the EMEA time zone.":                                                                          {-1, 4},
		"This is a remote role in Americas time zones.":                                                                              {-10, -3},
		"This role can be filled in European, Middle East and African time zones.":                                                   {-1, 4},
		"This role may be held anywhere in European, Middle Eastern, African or Americas regions or time zones.":                     {-10, 4},
		"This role requires strong working overlap with Americas time zones due to production support and on-call responsibilities.": {-10, -3},
		"This is a remote position and we're open to considering candidates located across EMEA and AMER time zones.":                {-10, 4},
	} {
		hours := readHours(Sentences(sentence))
		assert.Equal(t, HoursRequired, hours.Status, "%q", sentence)
		assert.Equal(t, want, [2]float64{hours.From, hours.To}, "%q", sentence)
		assert.NotEmpty(t, hours.Evidence, "%q", sentence)
	}
}

func TestReadHours_Preferences(t *testing.T) {
	t.Parallel()

	for sentence, want := range map[string][2]float64{
		"EST time zone is preferred for this position.":                                                       {-5, -5},
		"However, to facilitate working with the Finance team, we prefer candidates based in GMT+0 to GMT+3.": {0, 3},
		"We have a preference for candidates in EMEA time zones.":                                             {-1, 4},
		"Prefer location in PST, MST or CST timezone alignment.":                                              {-8, -6},
	} {
		hours := readHours(Sentences(sentence))
		assert.Equal(t, HoursPreferred, hours.Status, "%q", sentence)
		assert.Equal(t, want, [2]float64{hours.From, hours.To}, "%q", sentence)
	}
}

func TestReadHours_AsksNothing(t *testing.T) {
	t.Parallel()

	for _, sentence := range []string{
		"We're backend, frontend, and AI specialists working asynchronously across time zones, using issues, merge requests, and documentation as our main collaboration tools.",
		"Location: we have remote roles open in every time zone.",
		"We are growing teams in EMEA, Americas and APAC time zones, so can accommodate candidates from almost any country.",
		"A typical first-level software engineering team is based in a single time zone, such as EMEA or Americas, or APAC, with an Engineering Manager.",
		"Flexible working hours (fully flexible, as in most cases you only have to be on a couple of meetings weekly);",
		"Please note: applications will be open until Friday 2nd October 2026 at 08.00 CEST.",
		"Collaborate with enablement counterparts in the Americas and EMEA, and join the weekly team sync at 4pm CEST at least once per week.",
		"It’s a full-time position with 4 10-hour shifts, permanently covering weekends during EMEA Hours: 08:00-18:00 UTC",
		"As a 24 x 7 team, SOC analysts are expected to work Friday - Tuesday, with the daily shift being 0000 - 0800 UTC - (9am - 6pm AEST)",
		"You’re comfortable participating in a paid on-call rotation to help support urgent customer issues outside standard working hours.",
		"Develop a deep understanding of silicon partner strategies and identify overlap with Canonical’s current and future products.",
		"Canonical est une entreprise mondiale axée sur le travail à distance et elle est rentable.",
		"Join us in building the future.",
	} {
		hours := readHours(Sentences(sentence))
		assert.False(t, hours.Known(), "%q read as %+v", sentence, hours)
	}
}

func TestReadHours_JoinsAndRanks(t *testing.T) {
	t.Parallel()

	joined := readHours([]string{"8:00 AM - 4:00 PM EST", "11:00 AM - 7:00 PM EST"})
	assert.Equal(t, [2]float64{-7, -4}, [2]float64{joined.From, joined.To}, "a posting offering either schedule accepts both")
	assert.Equal(t, "8:00 AM - 4:00 PM EST", joined.Evidence, "the first sentence is the evidence")

	ranked := readHours([]string{"EST time zone is preferred.", "This role is home-based in EMEA time zones."})
	assert.Equal(t, HoursRequired, ranked.Status, "a requirement wins over a preference")
	assert.Equal(t, [2]float64{-1, 4}, [2]float64{ranked.From, ranked.To})
}

func TestWorkingHours_ShiftFor(t *testing.T) {
	t.Parallel()

	cet := WorkingHours{Status: HoursRequired, From: -2, To: 4}
	assert.Zero(t, cet.ShiftFor(1), "Lagos is inside CET ± 3")
	assert.Equal(t, 1.0, cet.ShiftFor(-3))
	assert.Equal(t, 1.5, cet.ShiftFor(5.5), "India is an hour and a half past it")

	eastern := WorkingHours{Status: HoursRequired, From: -9, To: -1}
	assert.Equal(t, 2.0, eastern.ShiftFor(1), "four hours with US Eastern is two hours' shift from Lagos")

	pacific := WorkingHours{Status: HoursRequired, From: -8, To: -8}
	assert.Equal(t, 4.0, pacific.ShiftFor(12), "measured around the clock: UTC+12 is four hours from UTC−8")
	assert.Equal(t, 9.0, pacific.ShiftFor(1))

	apac := WorkingHours{Status: HoursRequired, From: 5, To: 13}
	assert.Zero(t, apac.ShiftFor(-11), "UTC−11 is UTC+13 on the other side of the date line")
}

func TestReadSignals_ReadsHours(t *testing.T) {
	t.Parallel()

	job := Job{
		Title:               "Senior Backend Developer",
		Locations:           []string{"Anywhere in the World"},
		RemoteType:          RemoteTypeRemote,
		DescriptionMarkdown: "We build things.\n\n- Located in CET timezone (+/- 3 hours).\n- Python",
	}
	ReadSignals(&job)

	assert.Equal(t, WorkableAnywhere, job.Workable.Scope, "where and when stay separate (spec D5)")
	assert.Equal(t, HoursRequired, job.Hours.Status)
	assert.Equal(t, [2]float64{-2, 4}, [2]float64{job.Hours.From, job.Hours.To})
}

func TestFormatOffset(t *testing.T) {
	t.Parallel()

	for offset, want := range map[float64]string{0: "UTC", 1: "UTC+1", -5: "UTC−5", 5.5: "UTC+5:30", 5.75: "UTC+5:45", -3.5: "UTC−3:30", 15: "UTC−9"} {
		assert.Equal(t, want, FormatOffset(offset), "%v", offset)
	}
	assert.Equal(t, "UTC−2 to UTC+4", WorkingHours{From: -2, To: 4}.Window())
	assert.Equal(t, "UTC−5", WorkingHours{From: -5, To: -5}.Window())
}
