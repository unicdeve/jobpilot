package geo

import "sort"

// regionDefinition is a named region: the continents it spans, with
// countries added and taken away. Continents are GeoNames'; a region that
// is a business convention rather than a continent — EMEA, APAC, LATAM —
// is written out, since no standard says which countries it holds.
type regionDefinition struct {
	name       string
	aliases    []string
	continents []string
	include    []string
	exclude    []string
}

var (
	middleEast = []string{
		"AE", "BH", "EG", "IL", "IQ", "IR", "JO", "KW", "LB", "OM", "PS", "QA", "SA", "SY", "TR", "YE",
	}
	northAfrica    = []string{"DZ", "EG", "EH", "LY", "MA", "SD", "TN"}
	centralAmerica = []string{"BZ", "CR", "GT", "HN", "NI", "PA", "SV"}
	euEEA          = []string{
		"AT", "BE", "BG", "CY", "CZ", "DE", "DK", "EE", "ES", "FI", "FR", "GR", "HR", "HU", "IE",
		"IT", "LT", "LU", "LV", "MT", "NL", "PL", "PT", "RO", "SE", "SI", "SK",
		// The EEA and Switzerland, whom "EU work authorisation" also admits.
		"IS", "LI", "NO", "CH",
	}
)

// regionDefinitions are the regions postings were measured to name. A
// region's name is matched after folding, with its aliases.
//
// North America includes Mexico. Postings rarely say, and a candidate in
// Mexico shown a job meant for the United States and Canada loses less than
// one never shown a job meant for them (ADR-0023 §1).
var regionDefinitions = []regionDefinition{
	{name: "Europe", aliases: []string{"European"}, continents: []string{"EU"}},
	{name: "EU", aliases: []string{"European Union", "EEA", "EU/EEA"}, include: euEEA},
	{name: "EMEA", continents: []string{"EU", "AF"}, include: middleEast},
	{name: "Middle East", include: middleEast},
	{name: "MENA", include: append(append([]string{}, middleEast...), northAfrica...)},
	{name: "GCC", include: []string{"AE", "BH", "KW", "OM", "QA", "SA"}},
	{name: "Africa", continents: []string{"AF"}},
	{name: "Sub-Saharan Africa", continents: []string{"AF"}, exclude: northAfrica},
	{name: "North Africa", include: northAfrica},
	{name: "West Africa", aliases: []string{"Western Africa"}, include: []string{
		"BF", "BJ", "CI", "CV", "GH", "GM", "GN", "GW", "LR", "ML", "MR", "NE", "NG", "SL", "SN", "TG",
	}},
	{name: "East Africa", aliases: []string{"Eastern Africa"}, include: []string{
		"BI", "DJ", "ER", "ET", "KE", "KM", "MG", "MU", "MW", "MZ", "RW", "SC", "SO", "SS", "TZ", "UG", "ZM", "ZW",
	}},
	{name: "Southern Africa", include: []string{"BW", "LS", "NA", "SZ", "ZA"}},
	{name: "Asia", continents: []string{"AS"}},
	{name: "APAC", aliases: []string{"Asia Pacific", "Asia-Pacific", "Asia and Pacific"},
		continents: []string{"AS", "OC"}, exclude: middleEast},
	{name: "Southeast Asia", aliases: []string{"South East Asia", "South-East Asia"}, include: []string{
		"BN", "ID", "KH", "LA", "MM", "MY", "PH", "SG", "TH", "TL", "VN",
	}},
	{name: "South Asia", include: []string{"AF", "BD", "BT", "IN", "LK", "MV", "NP", "PK"}},
	{name: "Oceania", continents: []string{"OC"}},
	{name: "ANZ", aliases: []string{"Australasia"}, include: []string{"AU", "NZ"}},
	{name: "North America", aliases: []string{"Northern America", "NORAM", "NAMER"}, include: []string{"CA", "MX", "US"}},
	{name: "Central America", include: centralAmerica},
	{name: "South America", continents: []string{"SA"}},
	{name: "LATAM", aliases: []string{"LatAm", "Latin America"}, continents: []string{"SA"},
		include: append(append([]string{}, centralAmerica...), "CU", "DO", "HT", "MX", "PR")},
	{name: "Americas", aliases: []string{"The Americas", "AMER"}, continents: []string{"NA", "SA"}},
	{name: "DACH", include: []string{"AT", "CH", "DE"}},
	{name: "Nordics", aliases: []string{"Nordic", "Nordic countries"}, include: []string{"DK", "FI", "IS", "NO", "SE"}},
	{name: "Scandinavia", include: []string{"DK", "NO", "SE"}},
	{name: "Benelux", include: []string{"BE", "LU", "NL"}},
	{name: "Baltics", aliases: []string{"Baltic states"}, include: []string{"EE", "LT", "LV"}},
	{name: "CEE", aliases: []string{"Central and Eastern Europe", "Eastern Europe", "Central Eastern Europe"}, include: []string{
		"AL", "BA", "BG", "BY", "CZ", "EE", "HR", "HU", "LT", "LV", "MD", "ME", "MK", "PL", "RO", "RS", "SI", "SK", "UA", "XK",
	}},
}

// countries expands the region into its countries' codes, sorted.
func (r regionDefinition) countries(t *geoTables) []string {
	set := make(map[string]bool)
	for _, country := range generatedCountries {
		for _, continent := range r.continents {
			if country.Continent == continent {
				set[country.Code] = true
			}
		}
	}
	for _, code := range r.include {
		if _, ok := t.byCode[code]; ok {
			set[code] = true
		}
	}
	for _, code := range r.exclude {
		delete(set, code)
	}
	codes := make([]string, 0, len(set))
	for code := range set {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}
