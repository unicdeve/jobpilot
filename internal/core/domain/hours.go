package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// HoursStatus is whether a posting requires its working hours or only
// prefers them (docs/02_features/18_time_zones.md D4).
type HoursStatus string

// The statuses working hours can have. Empty is a posting that asks
// nothing about when the candidate works.
const (
	HoursRequired  HoursStatus = "required"
	HoursPreferred HoursStatus = "preferred"
)

// WorkingHours is the time zones a posting asks the candidate to work in,
// as one window of UTC offsets, in hours without daylight saving: "CET
// (+/- 3 hours)" is From −2, To +4 (ADR-0025 §1). To may pass +14 for a
// window that crosses the date line.
type WorkingHours struct {
	Status   HoursStatus `json:"status"`
	From     float64     `json:"from"`
	To       float64     `json:"to"`
	Evidence string      `json:"evidence,omitempty"`
}

// Known reports whether the posting says anything about its hours.
func (h WorkingHours) Known() bool {
	return h.Status == HoursRequired || h.Status == HoursPreferred
}

// ShiftFor is how many hours a candidate whose zone is offset would have to
// move their working day to be inside the window: zero inside it. It is
// measured around the clock, so UTC+12 is an hour from UTC−11.
func (h WorkingHours) ShiftFor(offset float64) float64 {
	width := h.To - h.From
	into := math.Mod(offset-h.From, 24)
	if into < 0 {
		into += 24
	}
	if into <= width {
		return 0
	}
	return math.Min(into-width, 24-into)
}

// maxWindow is the widest window kept. A posting that names nearly every
// zone — "teams in EMEA, Americas and APAC time zones" — asks nothing.
const maxWindow = 20

// workDay is the hours a candidate's own day is taken to run, in their own
// time: an overlap requirement is read against it.
const workDayStart, workDayEnd = 9.0, 17.0

// namedZones are the zone abbreviations postings write, at their standard
// offsets. They are matched in capitals only: "ET" is French for "and",
// and "est" is Latin for "is".
var namedZones = map[string]float64{
	"UTC": 0, "GMT": 0, "WET": 0, "WEST": 0, "BST": 0,
	"CET": 1, "CEST": 1, "EET": 2, "EEST": 2, "MSK": 3,
	"IST": 5.5, "SGT": 8, "AWST": 8, "JST": 9, "KST": 9, "AEST": 10, "AEDT": 10, "NZST": 12,
	"ET": -5, "EST": -5, "EDT": -5, "CT": -6, "CST": -6, "CDT": -6,
	"MT": -7, "MST": -7, "MDT": -7, "PT": -8, "PST": -8, "PDT": -8,
	"AKST": -9, "HST": -10, "BRT": -3, "ART": -3,
}

// zoneWords are the zones written out, longest first so "Central European"
// is not read as US Central. They are matched in any case, but only beside
// "time" or "zone", or as a coast.
var zoneWords = func() []zoneWord {
	var words []zoneWord
	for _, zone := range []struct {
		words  string
		offset float64
	}{
		{"central european", 1}, {"eastern european", 2}, {"western european", 0}, {"greenwich mean", 0},
		{"us eastern", -5}, {"us central", -6}, {"us mountain", -7}, {"us pacific", -8},
		{"eastern", -5}, {"central", -6}, {"mountain", -7}, {"pacific", -8},
		{"east coast", -5}, {"west coast", -8}, {"east", -5}, {"west", -8},
	} {
		pattern := `\b` + zone.words + `\b([\s-]+(standard\s+)?(time|zone|timezone|time zone)|\s*\(|,|\s+or\b)`
		if strings.HasSuffix(zone.words, "coast") {
			pattern = `\b` + zone.words + `\b`
		}
		words = append(words, zoneWord{regexp.MustCompile(pattern), zone.offset})
	}
	return words
}()

type zoneWord struct {
	pattern *regexp.Regexp
	offset  float64
}

var middleEast = regexp.MustCompile(`\bmiddle east(ern)?\b`)

// offsetZone is UTC or GMT with an offset written after it: "GMT+7".
var offsetZone = regexp.MustCompile(`\b(?:UTC|GMT)\s*([+\-−]\s*\d{1,2}(?:[:.]\d\d)?)`)

// zoneAbbreviation is a word that may be a zone's abbreviation.
var zoneAbbreviation = regexp.MustCompile(`\b[A-Z]{2,4}\b`)

// zoneRegions are the regions postings name as time zones, as windows of
// standard offsets: "home based in the EMEA time zone".
var zoneRegions = []struct {
	pattern  *regexp.Regexp
	from, to float64
}{
	{regexp.MustCompile(`(?i)\b(emea|europe,? (the )?middle east,? (and|&) africa|european,? middle eastern,? (and|or) african)\b`), -1, 4},
	{regexp.MustCompile(`(?i)\b(western europe(an)?)\b`), 0, 1},
	{regexp.MustCompile(`(?i)\b(europe|european)\b|\bEU\b`), -1, 3},
	{regexp.MustCompile(`(?i)\b(north america|north american|namer)\b`), -10, -3.5},
	// "us" is matched in capitals only, since it is also the pronoun.
	{regexp.MustCompile(`\b(US|U\.S\.)|(?i:\b(united states|american)\b)`), -10, -5},
	{regexp.MustCompile(`(?i)\b(latam|latin america|south america|south american)\b`), -8, -3},
	{regexp.MustCompile(`(?i)\b(americas|amer|north (or|and|&) south america(n)?)\b`), -10, -3},
	{regexp.MustCompile(`(?i)\b(apac|apj|asia[- ]pacific)\b`), 5, 13},
	{regexp.MustCompile(`(?i)\b(anz|australia|new zealand)\b`), 8, 12},
	{regexp.MustCompile(`(?i)\b(asia|asian)\b`), 5, 9},
	{regexp.MustCompile(`(?i)\b(africa|african)\b`), -1, 4},
	{regexp.MustCompile(`(?i)\b(middle east|middle eastern)\b`), 2, 4},
	{regexp.MustCompile(`(?i)\b(india|indian)\b`), 5.5, 5.5},
	{regexp.MustCompile(`(?i)\b(canada|canadian)\b`), -8, -3.5},
}

var (
	// hoursNoise are sentences about time zones that ask nothing: a team's
	// spread, a benefit, how people collaborate. It is checked only after
	// the readings that need numbers, since "CET (+/- 3 hours), we are
	// unable to consider candidates in other time zones" is a requirement.
	hoursNoise = regexp.MustCompile(`(?i)\b(every|all|any|multiple|different|various|several|many|disparate|other)\b[^.]{0,30}\btime ?zones?\b|\b(collaborat\w*|work\w*|communicat\w*|coordinat\w*|operat\w*|distributed|spread|spans?|teams?|connect\w*|async\w*)\b[^.]{0,25}\bacross\b[^.]{0,30}\btime ?zones?\b|\btime ?zones?[^.]{0,20}\b(apart|away|gap)\b|\bflexib\w*\b[^.]{0,20}\bhours\b|\boutside (of )?(standard |normal |regular )?(working|business) hours\b`)
	// shiftWork is a rota rather than a working day, which is shown as the
	// posting wrote it and not read (spec 18, out of scope).
	shiftWork = regexp.MustCompile(`(?i)\b(daily|night|rotating|\d+[- ]hour) shifts?\b|\bshifts? (being|is|will be|pattern|covers?|covering)\b|\bcovering weekends\b|\b24 ?x ?7\b`)
	// requireWords make a sentence naming zones a requirement without a
	// verb of place: "ONLY residents of countries with US time zones".
	requireWords = regexp.MustCompile(`(?i)\b(only|residents?|required|requires?|must|mandatory|need to|overlap\w*)\b`)
	// preferWords make a requirement a preference.
	preferWords = regexp.MustCompile(`(?i)\b(prefer|preferred|preferably|preference|ideally|ideal|nice to have|bonus|plus|encourage[ds]?)\b`)
	// regionTrigger is how a requirement places the candidate: "based in",
	// "remote in", "located in", "open to candidates in".
	regionTrigger = regexp.MustCompile(`(?i)\b(based|located|reside|residing|live|living|home[- ]based|remote|remotely|role|position|filled|held|open|available|must be|be|within|candidates|work|working|operate)\b[^.]{0,40}\b(in|within|from|across)\b|\boverlap\w*\b[^.]{0,30}\bwith\b`)

	offsetPart = `(?:[+\-−–]\s*\d{1,2}(?:[.:]\d{1,2})?|0)`
	// twoOffsets is a range of two offsets with UTC or GMT beside them:
	// "UTC−6 to UTC+3", "between UTC-4 and UTC+4", "+3 / -7 UTC",
	// "GMT to GMT+3".
	twoOffsets = regexp.MustCompile(`(?:\b(?:UTC|GMT)\s*(` + offsetPart + `)?|(` + offsetPart + `)\s*(?:UTC|GMT)\b)\s*(?:to|and|/|-|–|—|through)\s*(?:\b(?:UTC|GMT)\s*(` + offsetPart + `)?|(` + offsetPart + `)\s*(?:UTC|GMT)?\b)|(` + offsetPart + `)\s*/\s*(` + offsetPart + `)\s*(?:UTC|GMT)\b`)
	// zoneSpread is a zone and a margin: "CET (+/- 3 hours)", "UTC ± 2".
	zoneSpread = regexp.MustCompile(`\b([A-Z]{2,4})\b(?:\s*time ?zone)?\s*\(?\s*(?:\+/-|±|\+-|\+/−)\s*(\d{1,2})`)
	// withinOf is "within one hour of CET".
	withinOf = regexp.MustCompile(`(?i)\bwithin\s+(\w+)\s+hours?\s+of\s+`)
	// overlapHours is an overlap of a number of hours: "at least 4 hours
	// overlap", "overlap of 3-4 hours", "Overlap: 6hs".
	overlapHours = regexp.MustCompile(`(?i)\b(\w+)(?:\s*-\s*\d+)?\s*(?:hours?|hrs?|hs)\b[^.]{0,40}\boverlap|\boverlap\b[^.]{0,20}?\b(\w+)(?:\s*-\s*\d+)?\s*(?:hours?|hrs?|hs)\b`)
	// clockHours is a span of the clock in a zone: "9:00 AM – 12:00 PM
	// PST", "between 11:00 and 16:00 UTC", "10 AM - 6 PM EST".
	clockHours = regexp.MustCompile(`(?i)\b(\d{1,2})(?:[:.](\d\d))?\s*(am|pm|a\.m\.|p\.m\.)?\s*(?:-|–|—|to|and)\s*(\d{1,2})(?:[:.](\d\d))?\s*(am|pm|a\.m\.|p\.m\.)?\s+`)
	// zoneHours is working in a zone's day: "working hours in CEST",
	// "working EST hours", "CET business hours".
	zoneHours    = regexp.MustCompile(`(?i)\b(working|business|office|core)\s+hours\b|\bhours\s+(in|according to|aligned with)\b|\bwork(ing)?\s+\S+\s+(hours|time)\b|\bavailab\w*\s+(during|in)\b`)
	timeZoneWord = regexp.MustCompile(`(?i)\btime ?zones?\b`)
)

var numberWords = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
}

func number(word string) (float64, bool) {
	if n, ok := numberWords[strings.ToLower(word)]; ok {
		return n, true
	}
	n, err := strconv.ParseFloat(word, 64)
	return n, err == nil
}

// parseOffset reads "+3", "−5", "-3:30" or "0" as hours.
func parseOffset(text string) (float64, bool) {
	text = strings.NewReplacer("−", "-", "–", "-", " ", "").Replace(text)
	if text == "" {
		return 0, true
	}
	sign := 1.0
	switch text[0] {
	case '-':
		sign, text = -1, text[1:]
	case '+':
		text = text[1:]
	}
	hours, minutes, _ := strings.Cut(strings.ReplaceAll(text, ".", ":"), ":")
	h, err := strconv.ParseFloat(hours, 64)
	if err != nil || h > 14 {
		return 0, false
	}
	if minutes != "" {
		m, err := strconv.ParseFloat(minutes, 64)
		if err != nil {
			return 0, false
		}
		h += m / 60
	}
	return sign * h, true
}

// zonesIn returns the standard offsets of every zone a sentence names.
func zonesIn(sentence string) []float64 {
	var offsets []float64
	// "GMT+7" is the zone seven hours east of GMT, not GMT.
	for _, m := range offsetZone.FindAllStringSubmatch(sentence, -1) {
		if offset, ok := parseOffset(m[1]); ok {
			offsets = append(offsets, offset)
		}
	}
	sentence = offsetZone.ReplaceAllString(sentence, "")
	for _, word := range zoneAbbreviation.FindAllString(sentence, -1) {
		if offset, ok := namedZones[word]; ok {
			offsets = append(offsets, offset)
		}
	}
	// "Middle Eastern" is a region, not US Eastern time.
	lower := middleEast.ReplaceAllStringFunc(strings.ToLower(sentence), func(m string) string { return strings.Repeat(" ", len(m)) })
	for _, zone := range zoneWords {
		if loc := zone.pattern.FindStringIndex(lower); loc != nil {
			offsets = append(offsets, zone.offset)
			// A longer name has been read; its words are not read again.
			lower = lower[:loc[0]] + strings.Repeat(" ", loc[1]-loc[0]) + lower[loc[1]:]
		}
	}
	return offsets
}

type window struct{ from, to float64 }

func spanOf(offsets []float64) (window, bool) {
	if len(offsets) == 0 {
		return window{}, false
	}
	span := window{offsets[0], offsets[0]}
	for _, offset := range offsets[1:] {
		span.from, span.to = math.Min(span.from, offset), math.Max(span.to, offset)
	}
	return span, true
}

// windowIn reads the window one sentence asks for, if it asks for one.
func windowIn(sentence string) (window, bool) {
	if shiftWork.MatchString(sentence) {
		return window{}, false
	}
	if found, ok := exactWindow(sentence); ok {
		return found, true
	}
	if hoursNoise.MatchString(sentence) {
		return window{}, false
	}
	return namedWindow(sentence)
}

// exactWindow reads a window a sentence states with numbers: two offsets,
// a zone and a margin, an overlap of hours, or a span of the clock.
func exactWindow(sentence string) (window, bool) {
	if m := twoOffsets.FindStringSubmatch(sentence); m != nil {
		var offsets []float64
		for _, part := range []string{m[1] + m[2], m[3] + m[4], m[5], m[6]} {
			if part == "" {
				continue
			}
			if offset, ok := parseOffset(part); ok {
				offsets = append(offsets, offset)
			}
		}
		// "GMT to GMT+3": a bare GMT is zero.
		if len(offsets) == 1 && m[5] == "" {
			offsets = append(offsets, 0)
		}
		if len(offsets) == 2 {
			return spanOf(offsets)
		}
	}
	if m := zoneSpread.FindStringSubmatch(sentence); m != nil {
		if offset, ok := namedZones[m[1]]; ok {
			margin, _ := strconv.ParseFloat(m[2], 64)
			return window{offset - margin, offset + margin}, true
		}
	}
	zones := zonesIn(sentence)
	if m := withinOf.FindStringSubmatchIndex(sentence); m != nil {
		margin, ok := number(sentence[m[2]:m[3]])
		if after := zonesIn(sentence[m[1]:]); ok && len(after) > 0 {
			return window{after[0] - margin, after[0] + margin}, true
		}
	}
	if m := overlapHours.FindStringSubmatch(sentence); m != nil && len(zones) > 0 {
		n, ok := number(m[1] + m[2])
		if ok && n >= 1 && n <= 8 {
			span, _ := spanOf(zones)
			slack := (workDayEnd - workDayStart) - n
			return window{span.from - slack, span.to + slack}, true
		}
	}
	if m := clockHours.FindStringSubmatchIndex(sentence); m != nil && len(zones) > 0 {
		// The zone is the clock's only when it follows it: "9 AM – 12 PM
		// PST", not "3-4 hours of overlap with CET".
		next := sentence[m[1]:min(len(sentence), m[1]+14)]
		if hours, ok := clockSpan(sentence, m); ok {
			if after := zonesIn(next); len(after) > 0 {
				return hoursWindow(hours, after[0]), true
			}
		}
	}
	return window{}, false
}

// namedWindow reads a window a sentence names in words: zones, "working
// hours in CEST", or regions, "home based in EMEA time zones". Named zones
// are the narrower reading and win over regions beside them: "anywhere in
// the United States, however the ability to operate in ET, CT, MT, or PT
// timezones is required" asks for the four zones.
func namedWindow(sentence string) (window, bool) {
	zones := zonesIn(sentence)
	placed := timeZoneWord.MatchString(sentence) &&
		(regionTrigger.MatchString(sentence) || requireWords.MatchString(sentence) || preferWords.MatchString(sentence))
	if len(zones) > 0 && (zoneHours.MatchString(sentence) || placed || requireWords.MatchString(sentence)) {
		return spanOf(zones)
	}
	if !placed {
		return window{}, false
	}
	var offsets []float64
	for _, region := range zoneRegions {
		if region.pattern.MatchString(sentence) {
			offsets = append(offsets, region.from, region.to)
		}
	}
	return spanOf(offsets)
}

// clockSpan reads the start and end of a span of the clock, in 24-hour
// time, the end after the start.
func clockSpan(sentence string, m []int) ([2]float64, bool) {
	part := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return sentence[m[2*i]:m[2*i+1]]
	}
	read := func(hour, minute, meridiem string) (float64, bool) {
		h, err := strconv.ParseFloat(hour, 64)
		if err != nil || h > 24 {
			return 0, false
		}
		if minute != "" {
			mins, _ := strconv.ParseFloat(minute, 64)
			h += mins / 60
		}
		switch strings.ToLower(strings.ReplaceAll(meridiem, ".", "")) {
		case "pm":
			if h < 12 {
				h += 12
			}
		case "am":
			if h >= 12 {
				h -= 12
			}
		}
		return h, true
	}
	// "9 to 5" without am or pm is a working day; "9am - 5" is the same.
	startMeridiem, endMeridiem := part(3), part(6)
	if endMeridiem == "" && startMeridiem != "" {
		endMeridiem = "pm"
	}
	start, ok1 := read(part(1), part(2), startMeridiem)
	end, ok2 := read(part(4), part(5), endMeridiem)
	if !ok1 || !ok2 {
		return [2]float64{}, false
	}
	if startMeridiem == "" && endMeridiem == "" && end < start && end < 12 {
		end += 12
	}
	if end <= start {
		end += 24
	}
	return [2]float64{start, end}, true
}

// hoursWindow is where a candidate's own day must be for it to cover a span
// of hours in a zone: a day from 9 to 5 covers "9 to 12 Pacific" from
// UTC−8 to UTC−3. A span longer than the day is met by the one offset whose
// day is centred on it.
func hoursWindow(hours [2]float64, zone float64) window {
	from := zone + workDayStart - hours[0]
	to := zone + workDayEnd - hours[1]
	if from > to {
		mid := (from + to) / 2
		return window{mid, mid}
	}
	return window{from, to}
}

// readHours reads the working hours a posting asks for, sentence by
// sentence (ADR-0025 §1). A required window wins over a preferred one; two
// windows of the same kind are joined, since the posting accepts either.
func readHours(sentences []string) WorkingHours {
	var required, preferred WorkingHours
	for _, sentence := range sentences {
		found, ok := windowIn(sentence)
		if !ok || found.to-found.from >= maxWindow {
			continue
		}
		target := &required
		if preferWords.MatchString(sentence) {
			target = &preferred
		}
		if target.Status == "" {
			*target = WorkingHours{From: found.from, To: found.to, Evidence: evidence(sentence)}
			if target == &required {
				target.Status = HoursRequired
			} else {
				target.Status = HoursPreferred
			}
			continue
		}
		target.From, target.To = math.Min(target.From, found.from), math.Max(target.To, found.to)
	}
	if required.Known() {
		return required
	}
	return preferred
}

// FormatOffset writes an offset as a person reads it: "UTC+1", "UTC−5",
// "UTC+5:30", "UTC". An offset past +14 is written on the other side of the
// date line.
func FormatOffset(offset float64) string {
	if offset > 14 {
		offset -= 24
	}
	if offset == 0 {
		return "UTC"
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "−", -offset
	}
	hours := math.Floor(offset)
	text := "UTC" + sign + strconv.Itoa(int(hours))
	if minutes := math.Round((offset - hours) * 60); minutes > 0 {
		text += ":" + strconv.Itoa(int(minutes))
	}
	return text
}

// Window writes the hours' window: "UTC−2 to UTC+4", or "UTC+1" for one
// zone.
func (h WorkingHours) Window() string {
	if h.From == h.To {
		return FormatOffset(h.From)
	}
	return FormatOffset(h.From) + " to " + FormatOffset(h.To)
}
