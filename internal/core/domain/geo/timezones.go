package geo

import (
	"sort"
	"strings"
	"sync"
)

// TimeZone is an IANA time zone, the country it is in, and its offset from
// UTC in hours without daylight saving: Africa/Lagos is +1, America/New_York
// −5, Asia/Kolkata +5.5.
type TimeZone struct {
	ID      string  `json:"id"`
	Country string  `json:"country"`
	Offset  float64 `json:"offset"`
}

var timeZoneIndex = sync.OnceValue(func() map[string]TimeZone {
	index := make(map[string]TimeZone, len(generatedTimeZones))
	for _, zone := range generatedTimeZones {
		index[strings.ToLower(zone.ID)] = zone
	}
	return index
})

// TimeZoneByID returns the zone with this IANA name, in any case.
func TimeZoneByID(id string) (TimeZone, bool) {
	zone, ok := timeZoneIndex()[strings.ToLower(strings.TrimSpace(id))]
	return zone, ok
}

// TimeZones returns every zone, by name.
func TimeZones() []TimeZone {
	return append([]TimeZone(nil), generatedTimeZones...)
}

// TimeZonesOf returns a country's zones, west to east, so a country with
// one zone is one to choose and a country with several lists them in order.
func TimeZonesOf(country string) []TimeZone {
	country = strings.ToUpper(strings.TrimSpace(country))
	var zones []TimeZone
	for _, zone := range generatedTimeZones {
		if zone.Country == country {
			zones = append(zones, zone)
		}
	}
	sort.SliceStable(zones, func(i, j int) bool { return zones[i].Offset < zones[j].Offset })
	return zones
}
