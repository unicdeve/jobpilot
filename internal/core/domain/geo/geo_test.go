package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFold(t *testing.T) {
	t.Parallel()

	for written, want := range map[string]string{
		"Zürich":             "zurich",
		"  ZURICH ":          "zurich",
		"U.S.":               "us",
		"U.S.A.":             "usa",
		"München":            "munchen",
		"Villeneuve-d'Ascq":  "villeneuve d ascq",
		"São Paulo":          "sao paulo",
		"Remote (US)":        "remote us",
		"Türkiye":            "turkiye",
		"Hamburg,   Germany": "hamburg germany",
	} {
		assert.Equal(t, want, Fold(written), "%q", written)
	}
}

func TestLookup_Countries(t *testing.T) {
	t.Parallel()

	for written, code := range map[string]string{
		"Nigeria": "NG", "United States": "US", "USA": "US", "U.S.": "US", "US": "US",
		"United Kingdom": "GB", "UK": "GB", "England": "GB", "Deutschland": "DE",
		"The Netherlands": "NL", "Netherlands": "NL", "Holland": "NL", "Türkiye": "TR",
		"Czechia": "CZ", "Czech Republic": "CZ", "UAE": "AE", "Côte d'Ivoire": "CI",
		"Georgia": "GE", "Texas": "US", "Ontario": "CA", "Bavaria": "DE",
		"Congo": "CG", "Congo, The Democratic Republic of the": "CD", "Korea, Republic of": "KR",
	} {
		place, ok := Lookup(written, false)
		require.True(t, ok, "%q", written)
		assert.Equal(t, KindCountry, place.Kind, "%q", written)
		assert.Equal(t, []string{code}, place.Countries, "%q", written)
	}
}

func TestLookup_Regions(t *testing.T) {
	t.Parallel()

	emea, ok := Lookup("EMEA", false)
	require.True(t, ok)
	assert.Equal(t, KindRegion, emea.Kind)
	assert.Contains(t, emea.Countries, "NG", "Nigeria is in Africa, so in EMEA")
	assert.Contains(t, emea.Countries, "DE")
	assert.Contains(t, emea.Countries, "AE", "EMEA includes the Middle East")
	assert.NotContains(t, emea.Countries, "US")

	apac, _ := Lookup("Asia-Pacific", false)
	assert.Contains(t, apac.Countries, "AU")
	assert.Contains(t, apac.Countries, "SG")
	assert.NotContains(t, apac.Countries, "AE", "APAC leaves out the Middle East")

	latam, _ := Lookup("Latin America", false)
	assert.Contains(t, latam.Countries, "MX")
	assert.Contains(t, latam.Countries, "BR")
	assert.NotContains(t, latam.Countries, "US")

	northAmerica, _ := Lookup("Northern America", false)
	assert.Equal(t, []string{"CA", "MX", "US"}, northAmerica.Countries)

	eu, _ := Lookup("EU", false)
	assert.Contains(t, eu.Countries, "NO", "EU work authorisation admits the EEA")
	assert.NotContains(t, eu.Countries, "GB")

	for _, name := range []string{"Europe", "Africa", "West Africa", "Americas", "DACH", "Nordics", "APAC", "CEE"} {
		place, ok := Lookup(name, false)
		require.True(t, ok, name)
		assert.NotEmpty(t, place.Countries, name)
	}
}

func TestLookup_Cities(t *testing.T) {
	t.Parallel()

	for written, code := range map[string]string{
		"London": "GB", "Berlin": "DE", "München": "DE", "Munich": "DE", "Zürich": "CH",
		"Zurich": "CH", "Paris": "FR", "Lagos": "NG", "San Francisco": "US", "NYC": "US",
		"New York": "US", "Toronto": "CA", "Lille": "FR", "Utrecht": "NL", "Seattle": "US",
	} {
		place, ok := Lookup(written, true)
		require.True(t, ok, "%q", written)
		assert.Equal(t, KindCity, place.Kind, "%q", written)
		assert.Equal(t, []string{code}, place.Countries, "%q", written)
	}

	_, ok := Lookup("London", false)
	assert.False(t, ok, "reading prose, a city name alone is not a place")

	for _, word := range []string{"Remote", "Hybrid", "Onsite", "Worldwide", "Anywhere", "Global", "Full-time"} {
		_, ok := Lookup(word, true)
		assert.False(t, ok, "%q is not a place", word)
	}
}

func TestResolveCountry(t *testing.T) {
	t.Parallel()

	for written, code := range map[string]string{"Nigeria": "NG", "ng": "NG", "USA": "US", " Deutschland ": "DE"} {
		got, ok := ResolveCountry(written)
		require.True(t, ok, "%q", written)
		assert.Equal(t, code, got)
	}
	for _, written := range []string{"", "EMEA", "London", "Atlantis", "XX"} {
		_, ok := ResolveCountry(written)
		assert.False(t, ok, "%q is not a country", written)
	}
}

func TestEveryRegionCountryExists(t *testing.T) {
	t.Parallel()

	for _, region := range regionDefinitions {
		for _, code := range append(append([]string{}, region.include...), region.exclude...) {
			_, ok := CountryByCode(code)
			assert.True(t, ok, "%s names %s, which is not a country", region.name, code)
		}
	}
	for code := range countryAliases {
		_, ok := CountryByCode(code)
		assert.True(t, ok, "alias for unknown country %s", code)
	}
}

func TestLookup_ShortCityNames(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"Tor", "ACT", "SEA", "LON"} {
		_, ok := Lookup(code, true)
		assert.False(t, ok, "%q is an airport's code, not a city's name", code)
	}
	for written, name := range map[string]string{"NYC": "New York City", "SF": "San Francisco", "Ulm": "Ulm"} {
		place, ok := Lookup(written, true)
		require.True(t, ok, written)
		assert.Equal(t, name, place.Name)
	}
}

func TestSubdivisionOf(t *testing.T) {
	t.Parallel()

	country, ok := SubdivisionOf("Georgia")
	require.True(t, ok)
	assert.Equal(t, "US", country)

	place, ok := Lookup("Georgia", false)
	require.True(t, ok)
	assert.Equal(t, []string{"GE"}, place.Countries, "alone, Georgia is the country")

	_, ok = SubdivisionOf("Texas")
	assert.False(t, ok, "Texas is only a state")
}
