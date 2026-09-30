package geo

// countryAliases are the other ways postings and people write a country,
// beyond the name GeoNames gives it: abbreviations, the country's own
// name for itself, and the constituent countries of the United Kingdom.
//
// Two-letter codes are listed only where postings use them as words. "CA"
// is not here: after "San Francisco" it is California, not Canada.
//
// The ISO 3166 formal names are here too, as We Work Remotely writes them:
// "Korea (Republic of)", "Taiwan, Province of China". Read in pieces, the
// second would put a job in China.
var countryAliases = map[string][]string{
	"AE": {"UAE", "U.A.E.", "Emirates"},
	"AT": {"Österreich"},
	"BE": {"Belgique", "België"},
	"BN": {"Brunei Darussalam"},
	"BO": {"Bolivia (Plurinational State of)", "Bolivia, Plurinational State of"},
	"BR": {"Brasil"},
	"CD": {"DRC", "DR Congo", "Democratic Republic of the Congo", "Congo (Democratic Republic of the)", "Congo, The Democratic Republic of the", "Congo, Democratic Republic of the", "Congo-Kinshasa"},
	"CG": {"Congo", "Republic of Congo", "Congo-Brazzaville"},
	"CH": {"Schweiz", "Suisse", "Svizzera"},
	"CI": {"Côte d'Ivoire"},
	"CV": {"Cabo Verde"},
	"CZ": {"Czech Republic"},
	"DE": {"Deutschland"},
	"DK": {"Danmark"},
	"ES": {"España"},
	"FI": {"Suomi"},
	"FM": {"Micronesia (Federated States of)", "Micronesia, Federated States of"},
	"GB": {"UK", "U.K.", "Great Britain", "Britain", "England", "Scotland", "Wales", "Northern Ireland", "United Kingdom of Great Britain and Northern Ireland"},
	"IE": {"Republic of Ireland"},
	"IR": {"Iran (Islamic Republic of)", "Iran, Islamic Republic of"},
	"IT": {"Italia"},
	"KP": {"Korea (Democratic People's Republic of)", "Korea, Democratic People's Republic of"},
	"KR": {"Korea", "Republic of Korea", "Korea (Republic of)", "Korea, Republic of"},
	"LA": {"Lao People's Democratic Republic"},
	"MD": {"Moldova (Republic of)", "Moldova, Republic of"},
	"MK": {"Macedonia (the former Yugoslav Republic of)"},
	"MX": {"México"},
	"NL": {"Netherlands", "Holland"},
	"NO": {"Norge"},
	"PL": {"Polska"},
	"PS": {"Palestine", "Palestine, State of"},
	"RU": {"Russian Federation"},
	"SE": {"Sverige"},
	"SY": {"Syrian Arab Republic"},
	"SZ": {"Eswatini"},
	"TR": {"Türkiye"},
	"TW": {"Taiwan, Province of China"},
	"TZ": {"Tanzania, United Republic of"},
	"US": {"USA", "US", "U.S.", "U.S.A.", "United States of America", "the US", "the United States"},
	"VA": {"Holy See"},
	"VE": {"Venezuela (Bolivarian Republic of)", "Venezuela, Bolivarian Republic of"},
	"VG": {"Virgin Islands (British)", "Virgin Islands, British"},
	"VI": {"Virgin Islands (U.S.)", "Virgin Islands, U.S."},
	"VN": {"Viet Nam"},
}

// subdivisions are states and provinces postings name in place of their
// country: "Austin, Texas", "Toronto, Ontario". A subdivision never takes a
// name a country has, so "Georgia" stays the country.
var subdivisions = map[string][]string{
	"US": {
		"Alabama", "Alaska", "Arizona", "Arkansas", "California", "Colorado", "Connecticut",
		"Delaware", "District of Columbia", "Washington DC", "Florida", "Hawaii", "Idaho",
		"Illinois", "Indiana", "Iowa", "Kansas", "Kentucky", "Louisiana", "Maine", "Maryland",
		"Massachusetts", "Michigan", "Minnesota", "Mississippi", "Missouri", "Montana", "Nebraska",
		"Nevada", "New Hampshire", "New Jersey", "New Mexico", "New York State", "North Carolina",
		"North Dakota", "Ohio", "Oklahoma", "Oregon", "Pennsylvania", "Rhode Island",
		"South Carolina", "South Dakota", "Tennessee", "Texas", "Utah", "Vermont", "Virginia",
		"Washington State", "West Virginia", "Wisconsin", "Wyoming",
		"Bay Area", "San Francisco Bay Area", "Silicon Valley",
	},
	"CA": {
		"Ontario", "Quebec", "Québec", "British Columbia", "Alberta", "Manitoba", "Saskatchewan",
		"Nova Scotia", "New Brunswick", "Newfoundland and Labrador", "Prince Edward Island",
	},
	"DE": {
		"Bayern", "Bavaria", "Baden-Württemberg", "Nordrhein-Westfalen", "North Rhine-Westphalia",
		"Hessen", "Hesse", "Niedersachsen", "Lower Saxony", "Sachsen", "Saxony",
	},
	"AU": {"New South Wales", "Queensland", "Western Australia", "South Australia", "Tasmania"},
}
