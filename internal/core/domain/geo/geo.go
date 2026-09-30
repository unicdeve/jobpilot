// Package geo holds the countries, regions and cities JobPilot reads a job's
// location against (ADR-0023 §2).
//
// Everything here is compiled in. Placing a job needs no network and sends
// nothing anywhere: the countries and cities are generated from GeoNames
// (https://www.geonames.org/, licensed CC BY 4.0) by internal/gengeo, and the
// regions and alternative country names are written out in this package,
// from what postings were measured to say.
package geo

//go:generate go run ./internal/gengeo -out .

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"sort"
	"strings"
	"sync"
)

// Country is one country: its ISO 3166-1 alpha-2 code, its English short
// name, and the GeoNames continent it is on (AF, AN, AS, EU, NA, OC, SA).
type Country struct {
	Code      string
	Name      string
	Continent string
}

// Kind is what a place name named.
type Kind int

// The kinds of place a name can resolve to.
const (
	KindCountry Kind = iota + 1
	KindRegion
	KindCity
)

// Place is what a place name resolved to. Name is how the product writes
// it, whatever spelling it was found under. Countries is every country the
// place covers: one for a country or a city, many for a region.
//
// For a city, Elsewhere is the other countries with a city of that name,
// most populous first. Lookup picks the most populous; a caller that knows
// more, such as the state written after the name, can pick again with In.
type Place struct {
	Kind      Kind
	Name      string
	Countries []string
	Elsewhere []string
}

// In returns the city of this name in country, if there is one.
func (p Place) In(country string) (Place, bool) {
	if p.Kind != KindCity {
		return Place{}, false
	}
	if len(p.Countries) == 1 && p.Countries[0] == country {
		return p, true
	}
	for _, other := range p.Elsewhere {
		if other == country {
			return Place{Kind: KindCity, Name: p.Name, Countries: []string{country}}, true
		}
	}
	return Place{}, false
}

// CountryByCode returns the country with this ISO code, in any case.
func CountryByCode(code string) (Country, bool) {
	country, ok := tables().byCode[strings.ToUpper(strings.TrimSpace(code))]
	return country, ok
}

// Countries returns every country, ordered by name.
func Countries() []Country {
	all := append([]Country(nil), generatedCountries...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// ResolveCountry reads a country as a person writes it — "Nigeria",
// "USA", "Deutschland", or its ISO code — and returns its code.
//
// It is how a candidate's own words about a country, in their profile or a
// work authorization, meet the codes jobs are stored with. A region or a
// city is not a country, and resolves to nothing here.
func ResolveCountry(name string) (string, bool) {
	key := Fold(name)
	if country, ok := tables().countryNames[key]; ok {
		return country, true
	}
	if len(key) == 2 {
		if country, ok := CountryByCode(key); ok {
			return country.Code, true
		}
	}
	return "", false
}

// Lookup resolves a place name, folded or not, to a region, a country or a
// city, in that order: "Georgia" is the country before it is anything
// else, and a region's name is never a city's.
//
// Cities are looked up only when cities is true. A city name alone is often
// a word ("Nice", "Mobile", "Reading"), so a caller reading prose rather
// than a location field leaves them out.
func Lookup(name string, cities bool) (Place, bool) {
	t := tables()
	key := Fold(name)
	if region, ok := t.regions[key]; ok {
		return region, true
	}
	if code, ok := t.countryNames[key]; ok {
		return countryPlace(t, code), true
	}
	if !cities {
		return Place{}, false
	}
	if city, ok := t.cities()[key]; ok {
		return Place{Kind: KindCity, Name: city.name, Countries: city.countries[:1], Elsewhere: city.countries[1:]}, true
	}
	return Place{}, false
}

// Compounds are the names of countries and regions that hold a word or a
// mark a location is split at — "Bosnia and Herzegovina", "Taiwan, Province
// of China", "Central and Eastern Europe" — longest first. A reader
// splitting a location keeps these whole.
func Compounds() []string {
	return tables().compounds
}

// SubdivisionOf returns the country in which a country's name is also a
// state's: "Georgia" is a state of the US. A reader decides which the name
// means from what is written beside it.
func SubdivisionOf(name string) (string, bool) {
	code, ok := tables().alsoSubdivision[Fold(name)]
	return code, ok
}

// CountryPlace returns the country with this code as a place.
func CountryPlace(code string) (Place, bool) {
	t := tables()
	if _, ok := t.byCode[code]; !ok {
		return Place{}, false
	}
	return countryPlace(t, code), true
}

// CountryName returns the product's name for a country code, or the code
// itself when it is not one.
func CountryName(code string) string {
	if country, ok := CountryByCode(code); ok {
		return country.Name
	}
	return code
}

func countryPlace(t *geoTables, code string) Place {
	return Place{Kind: KindCountry, Name: t.byCode[code].Name, Countries: []string{code}}
}

//go:embed cities.tsv.gz
var citiesData []byte

type geoTables struct {
	byCode       map[string]Country
	countryNames map[string]string
	regions      map[string]Place
	compounds    []string
	cities       func() map[string]cityEntry
	// alsoSubdivision is the country each country name that is also a
	// state's is a state of: "georgia" is "US".
	alsoSubdivision map[string]string
}

// cityEntry is every city one folded name names.
type cityEntry struct {
	name      string
	countries []string
}

// tables builds the lookup maps once. The city table is decompressed on
// first use: most calls resolve a country or a region and never need it.
var tables = sync.OnceValue(func() *geoTables {
	t := &geoTables{
		byCode:       make(map[string]Country, len(generatedCountries)),
		countryNames: make(map[string]string, len(generatedCountries)*2),
		regions:      make(map[string]Place),

		alsoSubdivision: make(map[string]string),
	}
	for _, country := range generatedCountries {
		t.byCode[country.Code] = country
		t.countryNames[Fold(country.Name)] = country.Code
	}
	for code, names := range countryAliases {
		for _, name := range names {
			t.countryNames[Fold(name)] = code
		}
	}
	for code, names := range subdivisions {
		for _, name := range names {
			if _, taken := t.countryNames[Fold(name)]; taken {
				t.alsoSubdivision[Fold(name)] = code
				continue
			}
			t.countryNames[Fold(name)] = code
		}
	}
	for _, region := range regionDefinitions {
		place := Place{Kind: KindRegion, Name: region.name, Countries: region.countries(t)}
		for _, name := range append([]string{region.name}, region.aliases...) {
			t.regions[Fold(name)] = place
		}
	}
	t.compounds = compounds()
	t.cities = sync.OnceValue(func() map[string]cityEntry { return readCities(t) })
	return t
})

// compounds collects every country and region name that a location would
// be split inside.
func compounds() []string {
	var names []string
	add := func(name string) {
		if strings.ContainsAny(name, ",()&/") || strings.Contains(strings.ToLower(name), " and ") {
			names = append(names, name)
		}
	}
	for _, country := range generatedCountries {
		add(country.Name)
	}
	for _, aliases := range countryAliases {
		for _, name := range aliases {
			add(name)
		}
	}
	for _, region := range regionDefinitions {
		for _, name := range append([]string{region.name}, region.aliases...) {
			add(name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	return names
}

// readCities decompresses the embedded city table. A name that is also a
// country's or a region's is left to it.
func readCities(t *geoTables) map[string]cityEntry {
	zr, err := gzip.NewReader(bytes.NewReader(citiesData))
	if err != nil {
		panic("geo: the embedded city table is unreadable: " + err.Error())
	}
	cities := make(map[string]cityEntry, 70_000)
	scanner := bufio.NewScanner(zr)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		name := fields[0]
		if _, isCountry := t.countryNames[name]; isCountry {
			continue
		}
		if _, isRegion := t.regions[name]; isRegion {
			continue
		}
		cities[name] = cityEntry{name: fields[2], countries: strings.Split(fields[1], ",")}
	}
	if err := scanner.Err(); err != nil {
		panic("geo: the embedded city table is unreadable: " + err.Error())
	}
	for abbreviation, name := range cityAbbreviations {
		if city, ok := cities[Fold(name)]; ok {
			cities[Fold(abbreviation)] = city
		}
	}
	return cities
}
