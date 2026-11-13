package platform

import (
	"math"
	"testing"
)

func TestActualCoordinateConversion(t *testing.T) {
	p, ok := PlatformLocation(UsageLocation{Latitude: 39.915, Longitude: 116.404}, "wgs84")
	if !ok || math.Abs(p.Longitude-116.41024449916938) > 1e-8 || math.Abs(p.Latitude-39.91640428150164) > 1e-8 {
		t.Fatal("WGS84 to GCJ02 conversion changed", p)
	}
	for _, system := range []string{"", "gcj02"} {
		q, ok := PlatformLocation(p, system)
		if !ok || q != p {
			t.Fatal("already GCJ02 converted twice")
		}
	}
	outside := UsageLocation{Latitude: 51.5, Longitude: -.1}
	if q, ok := PlatformLocation(outside, "wgs84"); !ok || q != outside {
		t.Fatal("coordinates outside China changed")
	}
	for _, p := range []UsageLocation{{}, {Latitude: 91, Longitude: 1}, {Latitude: math.NaN(), Longitude: 1}} {
		if _, ok := PlatformLocation(p, "wgs84"); ok {
			t.Fatal("invalid actual coordinates accepted")
		}
	}
	if _, ok := PlatformLocation(p, "school"); ok {
		t.Fatal("school selection treated as actual coordinates")
	}
}
