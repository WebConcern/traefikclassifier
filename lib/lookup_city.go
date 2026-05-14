package lib

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	geoip2 "github.com/WebConcern/traefikclassifier/geoip2"
	geoip2_iso88591 "github.com/WebConcern/traefikclassifier/geoip2_iso88591"
)

var (
	lookupCityMu       sync.Mutex
	lookupCityInstance LookupGeoIPCity
)

// GeoIPCityResult in memory, this should have between 126 and 180 bytes. On average, consider 150 bytes.
type GeoIPCityResult struct {
	country        string
	countryCode    string
	region         string
	regionCode     string
	city           string
	latitude       string
	longitude      string
	accuracyRadius string
	geohash        string
	postalCode     string
}

const kmToMeters = 1000

// LookupGeoIPCity LookupGeoIP.
type LookupGeoIPCity func(ip net.IP) (*GeoIPCityResult, error)

// CreateCityDBLookup CreateCityDBLookup.
func CreateCityDBLookup(rdr *geoip2.CityReader) LookupGeoIPCity {
	return func(ip net.IP) (*GeoIPCityResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPCityResult{
			country:        Unknown,
			countryCode:    rec.Country.ISOCode,
			region:         Unknown,
			regionCode:     Unknown,
			city:           Unknown,
			postalCode:     rec.Postal.Code,
			latitude:       strconv.FormatFloat(rec.Location.Latitude, 'f', -1, 64),
			longitude:      strconv.FormatFloat(rec.Location.Longitude, 'f', -1, 64),
			accuracyRadius: strconv.Itoa(int(rec.Location.AccuracyRadius) * kmToMeters),
			geohash:        EncodeGeoHash(rec.Location.Latitude, rec.Location.Longitude),
		}
		if country, ok := rec.Country.Names["en"]; ok {
			returnVal.country = country
		}
		if city, ok := rec.City.Names["en"]; ok {
			returnVal.city = city
		}
		if rec.Subdivisions != nil {
			if region, ok := rec.Subdivisions[0].Names["en"]; ok {
				returnVal.region = region
			}
			returnVal.regionCode = rec.Subdivisions[0].ISOCode
		}
		return &returnVal, nil
	}
}

// CreateCityDBLookupIso88591 CreateCityDBLookup.
func CreateCityDBLookupIso88591(rdr *geoip2_iso88591.CityReader) LookupGeoIPCity {
	return func(ip net.IP) (*GeoIPCityResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPCityResult{
			country:        Unknown,
			countryCode:    rec.Country.ISOCode,
			region:         Unknown,
			regionCode:     Unknown,
			city:           Unknown,
			postalCode:     rec.Postal.Code,
			latitude:       strconv.FormatFloat(rec.Location.Latitude, 'f', -1, 64),
			longitude:      strconv.FormatFloat(rec.Location.Longitude, 'f', -1, 64),
			accuracyRadius: strconv.Itoa(int(rec.Location.AccuracyRadius) * kmToMeters),
			geohash:        EncodeGeoHash(rec.Location.Latitude, rec.Location.Longitude),
		}
		if country, ok := rec.Country.Names["en"]; ok {
			returnVal.country = country
		}
		if city, ok := rec.City.Names["en"]; ok {
			returnVal.city = city
		}
		if rec.Subdivisions != nil {
			if region, ok := rec.Subdivisions[0].Names["en"]; ok {
				returnVal.region = region
			}
			returnVal.regionCode = rec.Subdivisions[0].ISOCode
		}
		return &returnVal, nil
	}
}

// NewLookupCity returns the shared city lookup singleton, creating it on the first call.
func NewLookupCity(dbPath, name string, iso88591 bool) (LookupGeoIPCity, error) {
	lookupCityMu.Lock()
	defer lookupCityMu.Unlock()

	if lookupCityInstance != nil {
		return lookupCityInstance, nil
	}

	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("city DB not found: db=%s, name=%s, err=%w", dbPath, name, err)
	}

	if iso88591 {
		rdr, err := geoip2_iso88591.NewCityReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("city lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupCityInstance = CreateCityDBLookupIso88591(rdr)
	} else {
		rdr, err := geoip2.NewCityReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("city lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupCityInstance = CreateCityDBLookup(rdr)
	}
	return lookupCityInstance, nil
}

// ResetLookupCity clears the singleton for testing.
func ResetLookupCity() {
	lookupCityMu.Lock()
	defer lookupCityMu.Unlock()
	lookupCityInstance = nil
}
