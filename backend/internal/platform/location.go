package platform

import "math"

// PlatformLocation converts a real OS WGS84 fix to the GCJ02 coordinates
// used by the original AMap client. Legacy clients already send GCJ02.
func PlatformLocation(p UsageLocation, system string) (UsageLocation, bool) {
	if !p.Valid() || (system != "" && system != "gcj02" && system != "wgs84") {
		return UsageLocation{}, false
	}
	if system != "wgs84" || p.Longitude < 72.004 || p.Longitude > 137.8347 || p.Latitude < 0.8293 || p.Latitude > 55.8271 {
		return p, true
	}
	x, y := p.Longitude-105, p.Latitude-35
	dlat := -100 + 2*x + 3*y + .2*y*y + .1*x*y + .2*math.Sqrt(math.Abs(x))
	dlat += (20*math.Sin(6*x*math.Pi) + 20*math.Sin(2*x*math.Pi)) * 2 / 3
	dlat += (20*math.Sin(y*math.Pi) + 40*math.Sin(y/3*math.Pi)) * 2 / 3
	dlat += (160*math.Sin(y/12*math.Pi) + 320*math.Sin(y*math.Pi/30)) * 2 / 3
	dlng := 300 + x + 2*y + .1*x*x + .1*x*y + .1*math.Sqrt(math.Abs(x))
	dlng += (20*math.Sin(6*x*math.Pi) + 20*math.Sin(2*x*math.Pi)) * 2 / 3
	dlng += (20*math.Sin(x*math.Pi) + 40*math.Sin(x/3*math.Pi)) * 2 / 3
	dlng += (150*math.Sin(x/12*math.Pi) + 300*math.Sin(x/30*math.Pi)) * 2 / 3
	rad := p.Latitude / 180 * math.Pi
	magic := 1 - .00669342162296594323*math.Sin(rad)*math.Sin(rad)
	sqrt := math.Sqrt(magic)
	dlat = dlat * 180 / ((6378245 * (1 - .00669342162296594323)) / (magic * sqrt) * math.Pi)
	dlng = dlng * 180 / (6378245 / sqrt * math.Cos(rad) * math.Pi)
	return UsageLocation{Latitude: p.Latitude + dlat, Longitude: p.Longitude + dlng}, true
}
