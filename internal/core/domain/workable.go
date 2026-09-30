package domain

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

// WorkableScope is how far a job says it can be done from (ADR-0023 §1).
type WorkableScope string

// The scopes a job can have. A job stored before ADR-0023 has none until
// the daemon reads it again, and is treated as unknown meanwhile.
const (
	// WorkableAnywhere is a job with no restriction on where the candidate is.
	WorkableAnywhere WorkableScope = "anywhere"
	// WorkableListed is a job open in these countries, or countries in these
	// regions; for an onsite or hybrid job, the office's.
	WorkableListed WorkableScope = "listed"
	// WorkableUnknown is a job whose source did not say, said something
	// JobPilot does not read, or labelled it in a way its own description
	// contradicts.
	WorkableUnknown WorkableScope = "unknown"
)

// Workable is where a job can be done from.
type Workable struct {
	Scope WorkableScope `json:"scope"`
	// Places are what the job's locations were read as, in the product's
	// words: "EMEA", "United States", "Berlin".
	Places []string `json:"places"`
	// Countries are the ISO codes Places cover, regions expanded, for a
	// listed job. A job anywhere or unknown has none.
	Countries []string `json:"countries"`
	// Note is the description's sentence that made the job unknown, when
	// one did.
	Note string `json:"note,omitempty"`
}

// OpenTo reports whether a candidate in country can take the job without
// moving, as far as the job says. Unknown is not a yes: callers keep an
// unknown job visible by rule, not because it is open to them.
func (w Workable) OpenTo(country string) bool {
	switch w.Scope {
	case WorkableAnywhere:
		return true
	case WorkableListed:
		return slices.Contains(w.Countries, strings.ToUpper(country))
	}
	return false
}

// Known reports whether the job says where it can be done from.
func (w Workable) Known() bool {
	return w.Scope == WorkableAnywhere || w.Scope == WorkableListed
}

var (
	// alternativeSeparators split a location into the alternatives it lists:
	// "London, UK or NYC", "Berlin | München". itemSeparators split one
	// alternative into its parts: "Cambridge, MA", "Zürich (Switzerland)".
	alternativeSeparators = regexp.MustCompile(`(?i)[;|/+&·]|\s[-–—]\s|\s(?:or|and|oder|ou)\s`)
	itemSeparators        = regexp.MustCompile(`[,()\[\]]`)
	// anywhereWords say a job has no restriction on where the candidate is.
	anywhereWords = regexp.MustCompile(`\b(worldwide|world wide|anywhere|global|globally|everywhere|international|internationally|all countries|any country|any location)\b`)
	// negationWords say a location is a list of where the job is not.
	// JobPilot does not read exclusions, so such a location is unknown.
	negationWords = regexp.MustCompile(`\b(not in|not from|excluding|exclude|except|outside)\b`)
	// timeZoneWords mark a time-zone requirement, which is not a place.
	timeZoneWords = regexp.MustCompile(`\b(time ?zones?|timezones?|hours|utc|gmt|cet|cest|est|edt|pst|pdt|pt|et|ct|overlap)\b`)
	// noiseWords say how or on what terms a job is done, not where.
	noiseWords = map[string]bool{
		"remote": true, "remotely": true, "fully": true, "full": true, "100": true, "only": true,
		"hybrid": true, "onsite": true, "on": true, "site": true, "office": true, "in": true,
		"based": true, "candidates": true, "applicants": true, "residents": true, "citizens": true,
		"persons": true, "preferred": true, "required": true, "work": true, "from": true,
		"home": true, "wfh": true, "flexible": true, "location": true, "locations": true,
		"the": true, "within": true, "remoto": true, "distance": true, "a": true, "poste": true,
	}
)

// locationReading is what one location string said.
type locationReading struct {
	anywhere bool
	places   []geo.Place
}

// readLocation reads one location as a source wrote it.
//
// It recognises the words for anywhere, and region, country and city
// names; everything else is left unread rather than guessed at. A location
// that lists where a job is not — "not in the US, CA, UK" — is unread as a
// whole, since reading only its places would open the job exactly where it
// is closed.
func readLocation(location string) locationReading {
	folded := geo.Fold(location)
	if negationWords.MatchString(folded) {
		return locationReading{}
	}
	var reading locationReading
	reading.anywhere = anywhereWords.MatchString(folded)
	for _, alternative := range alternativeSeparators.Split(keepCompoundsWhole(location), -1) {
		reading.places = append(reading.places, readAlternative(alternative)...)
	}
	return reading
}

// compoundName matches any country or region name a location would be
// split inside, in any case and spacing.
var compoundName = sync.OnceValue(func() *regexp.Regexp {
	names := geo.Compounds()
	patterns := make([]string, len(names))
	for i, name := range names {
		words := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune(" ,()&/'", r) })
		for j, word := range words {
			words[j] = regexp.QuoteMeta(word)
		}
		patterns[i] = strings.Join(words, `[\s,()&/']+`)
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(patterns, "|") + `)\b\)?`)
})

// keepCompoundsWhole joins the words of a compound name with no-break
// spaces, which no separator splits at and which folding reads as spaces:
// "Bosnia and Herzegovina" stays one country rather than "Bosnia" and
// "Herzegovina", neither of which is one.
func keepCompoundsWhole(location string) string {
	return compoundName().ReplaceAllStringFunc(location, func(name string) string {
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune(" ,()&/", r) {
				return '\u00a0'
			}
			return r
		}, name)
	})
}

// readAlternative reads one alternative a location lists.
//
// What else the alternative names says which city a name means:
// "Cambridge, MA" is not Cambridge in England, and "London, ON" is in
// Canada. A state's code is read only this way, never as a place of its
// own, so "Hamburg, DE" stays in Germany rather than Delaware. It reaches no
// further than its own alternative: in "London, UK or NYC", the UK says
// nothing about New York.
func readAlternative(alternative string) []geo.Place {
	var read []geo.Place
	states := make(map[string]bool)
	for _, item := range itemSeparators.Split(alternative, -1) {
		if country, ok := subdivisionCodes[strings.TrimSpace(item)]; ok {
			states[country] = true
			continue
		}
		read = append(read, readSegment(item)...)
	}
	read = placeStates(read, states)

	named := make(map[string]bool)
	for _, place := range read {
		if place.Kind != geo.KindCity {
			for _, country := range place.Countries {
				named[country] = true
			}
		}
	}
	// Cities are placed once the whole alternative is read, and kept in
	// the order it names them.
	places := make([]geo.Place, 0, len(read))
	for _, place := range read {
		if place.Kind == geo.KindCity {
			var ok bool
			if place, ok = placeCity(place, named, states); !ok {
				continue
			}
		}
		places = append(places, place)
	}
	return places
}

// placeStates reads a country's name that is also a state's as the state
// when the rest of the alternative is in the state's country: "Atlanta,
// Georgia" and "Remote, US, Georgia" are in the United States, while
// "Tbilisi, Georgia" is in Georgia.
func placeStates(read []geo.Place, states map[string]bool) []geo.Place {
	for i, place := range read {
		if place.Kind != geo.KindCountry {
			continue
		}
		country, ok := geo.SubdivisionOf(place.Name)
		if !ok {
			continue
		}
		if states[country] || elsewhereIn(read, i, country, place.Countries[0]) {
			if state, ok := geo.CountryPlace(country); ok {
				read[i] = state
			}
		}
	}
	return read
}

// elsewhereIn reports whether a place other than read[skip] is in country:
// a country or region that holds it, or a city that is there and not in
// other.
func elsewhereIn(read []geo.Place, skip int, country, other string) bool {
	for i, place := range read {
		if i == skip {
			continue
		}
		if place.Kind != geo.KindCity {
			if slices.Contains(place.Countries, country) {
				return true
			}
			continue
		}
		all := append(append([]string{}, place.Countries...), place.Elsewhere...)
		if slices.Contains(all, country) && !slices.Contains(all, other) {
			return true
		}
	}
	return false
}

// placeCity picks the city a name means from what the rest of its
// alternative names.
//
// A country or region named beside it decides: the city is the one there,
// and a city of that name in none of them is dropped, since "Paris, Texas"
// is not in France and the rest of the location already says where it is.
// A state's code only chooses among the cities the name has, and is
// otherwise not a state at all. With neither, the name is its most
// populous city.
func placeCity(city geo.Place, named, states map[string]bool) (geo.Place, bool) {
	if len(named) > 0 {
		return cityAmong(city, named)
	}
	if place, ok := cityAmong(city, states); ok {
		return place, true
	}
	return city, true
}

func cityAmong(city geo.Place, countries map[string]bool) (geo.Place, bool) {
	if countries[city.Countries[0]] {
		return city, true
	}
	for _, country := range city.Elsewhere {
		if countries[country] {
			return city.In(country)
		}
	}
	return geo.Place{}, false
}

// subdivisionCodes are the state and province codes postings write after
// a city, in capitals, as in "Seattle, WA".
var subdivisionCodes = func() map[string]string {
	codes := make(map[string]string)
	for _, code := range strings.Fields(`AL AK AZ AR CA CO CT DE DC FL GA HI ID IL IN IA KS KY LA ME MD
		MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY`) {
		codes[code] = "US"
	}
	for _, code := range strings.Fields(`AB BC MB NB NL NS ON PE QC SK`) {
		codes[code] = "CA"
	}
	return codes
}()

// readSegment reads one listed place. The whole segment, less the words
// that are not places, is tried first; then its runs of words, which is
// what finds "Belgium" in "Hybrid Belgium 3 days" and "Berlin" in "LiveEO
// GmbH Berlin Office". Cities count here, since a location field is where
// a posting names its city.
func readSegment(segment string) []geo.Place {
	folded := geo.Fold(segment)
	if folded == "" || timeZoneWords.MatchString(folded) {
		return nil
	}
	var words []string
	for _, word := range strings.Fields(folded) {
		if !noiseWords[word] {
			words = append(words, word)
		}
	}
	if len(words) == 0 {
		return nil
	}
	if place, ok := geo.Lookup(strings.Join(words, " "), true); ok {
		return []geo.Place{place}
	}
	return placesInWords(words, nil, true)
}

// placesInWords finds places in a run of words, longest first, each word
// used once; cities only when cities is set. original, when given, is the
// words as they were written, so that "us" the pronoun is not read as "US"
// the country.
func placesInWords(words, original []string, cities bool) []geo.Place {
	var places []geo.Place
	for start := 0; start < len(words); {
		matched := 0
		for n := min(4, len(words)-start); n > 0; n-- {
			candidate := strings.Join(words[start:start+n], " ")
			if n == 1 && candidate == "us" && original != nil && original[start] == "us" {
				continue
			}
			if place, ok := geo.Lookup(candidate, cities); ok {
				places = append(places, place)
				matched = n
				break
			}
		}
		if matched == 0 {
			matched = 1
		}
		start += matched
	}
	return places
}

// readWorkable derives where a job can be done from, from its locations
// and remote type, and checks the result against its description.
func readWorkable(job Job, sentences []string) Workable {
	inPlace := job.RemoteType == RemoteTypeOnsite || job.RemoteType == RemoteTypeHybrid

	anywhere := false
	var places []geo.Place
	for _, location := range job.Locations {
		reading := readLocation(location)
		// An onsite job is done where its office is, whatever else its
		// location says.
		if reading.anywhere && !inPlace {
			anywhere = true
		}
		places = append(places, reading.places...)
	}

	workable := Workable{Scope: WorkableUnknown}
	switch {
	case anywhere:
		workable.Scope = WorkableAnywhere
	case len(places) > 0:
		workable.Scope = WorkableListed
		workable.Places, workable.Countries = placeNames(places)
	}
	if inPlace || !workable.Known() {
		return workable
	}

	// The title and the description can make a job less certain, never
	// more (ADR-0023 §1). A title naming a place its label does not cover —
	// "Staff Backend Engineer (Europe)" on a job listed as worldwide — is
	// read the way a restricting sentence is.
	if named := countriesIn(job.Title); len(named) > 0 &&
		(workable.Scope == WorkableAnywhere || !overlaps(named, workable.Countries)) {
		return Workable{Scope: WorkableUnknown, Note: evidence(job.Title)}
	}
	if note, ok := contradiction(workable, sentences); ok {
		return Workable{Scope: WorkableUnknown, Note: note}
	}
	return workable
}

// placeNames returns the places' names and their countries, each once and
// in order.
func placeNames(places []geo.Place) ([]string, []string) {
	var names, countries []string
	for _, place := range places {
		if !slices.Contains(names, place.Name) {
			names = append(names, place.Name)
		}
		for _, country := range place.Countries {
			if !slices.Contains(countries, country) {
				countries = append(countries, country)
			}
		}
	}
	slices.Sort(countries)
	return names, countries
}

var (
	// residencePhrases require the candidate to be somewhere. They
	// contradict a job labelled anywhere whether or not a place is named:
	// "must reside in the San Francisco Bay Area" is a restriction even
	// where "Bay Area" is not read.
	residencePhrases = regexp.MustCompile(strings.Join([]string{
		`\b(must|need to|needs to|required to|have to|has to|should)( currently)?( be)? (located|based|residing|reside|living|live|resident)( in| within| out of)\b`,
		`\b(based|located|residing) in (the )?[a-z ]{2,30} only\b`,
		`\bbased (candidates |applicants |residents |talent )?only\b`,
		`\bonly (open to|accepting|considering|hiring|available to)\b.{0,40}\b(in|from|based|located|residing|living)\b`,
		`\bopen to (candidates|applicants|residents) (residing|located|based|living) in\b`,
		`\b(citizens?|persons|nationals) only\b`,
		`\btax residen(cy|t)\b`,
	}, "|"))
	// authorizationPhrases restrict where a candidate may work only when
	// they name a place: "authorized to work in the United States" does,
	// "eligible to work in the country you live in" does not.
	authorizationPhrases = regexp.MustCompile(strings.Join([]string{
		`\b(authori[sz]ed|eligible|legally (able|allowed|permitted)|permitted|entitled|able) to work (in|within|for)\b`,
		`\bright to work in\b`,
		`\bremote(ly)? (within|in|from) (the )?[a-z]`,
	}, "|"))
)

// contradiction finds the first description sentence that restricts where a
// job labelled anywhere or listed can be done from, beyond what its label
// says. A listed job whose sentence names any of its own places is
// consistent: "authorized to work in the United States" on a job listed for
// the United States restricts nothing further, and "must be within EMEA" on
// a job Himalayas lists as 149 countries of EMEA agrees with it, though the
// two lists of EMEA differ at their edges. Only a sentence naming none of
// the listed places says the listing is wrong.
func contradiction(workable Workable, sentences []string) (string, bool) {
	for _, sentence := range sentences {
		folded := geo.Fold(sentence)
		residence := residencePhrases.MatchString(folded)
		authorization := authorizationPhrases.MatchString(folded)
		if !residence && !authorization {
			continue
		}
		if timeZoneWords.MatchString(folded) {
			// "must be located within three hours of CET" is a time zone.
			continue
		}

		named := countriesIn(sentence)
		if len(named) == 0 && !residence {
			continue
		}
		if workable.Scope == WorkableListed && len(named) > 0 && overlaps(named, workable.Countries) {
			continue
		}
		return evidence(sentence), true
	}
	return "", false
}

// countriesIn is every country a sentence names, as a country or a region.
// Cities are not read in prose: "Nice to have" is not in France.
func countriesIn(sentence string) []string {
	var folded, original []string
	for _, word := range strings.FieldsFunc(sentence, func(r rune) bool {
		return r != '.' && r != '\'' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		f := geo.Fold(word)
		if f == "" {
			continue
		}
		folded = append(folded, f)
		original = append(original, strings.ReplaceAll(word, ".", ""))
	}
	_, countries := placeNames(placesInWords(folded, original, false))
	return countries
}

// overlaps reports whether any of countries is among of.
func overlaps(countries, of []string) bool {
	for _, country := range countries {
		if slices.Contains(of, country) {
			return true
		}
	}
	return false
}
