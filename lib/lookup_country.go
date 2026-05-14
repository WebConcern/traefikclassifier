package lib

import (
	"fmt"
	"net"
	"os"
	"sync"

	geoip2 "github.com/WebConcern/traefikclassifier/geoip2"
	geoip2_iso88591 "github.com/WebConcern/traefikclassifier/geoip2_iso88591"
)

var (
	lookupCountryMu       sync.Mutex
	lookupCountryInstance LookupGeoIPCountry
)

// GeoIPCountryResult in memory, this should have between 126 and 180 bytes. On average, consider 150 bytes.
type GeoIPCountryResult struct {
	country     string
	countryCode string
}

// LookupGeoIPCountry LookupGeoIPCountry.
type LookupGeoIPCountry func(ip net.IP) (*GeoIPCountryResult, error)

// CreateCountryDBLookup CreateCountryDBLookup.
func CreateCountryDBLookup(rdr *geoip2.CountryReader) LookupGeoIPCountry {
	return func(ip net.IP) (*GeoIPCountryResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPCountryResult{
			country:     Unknown,
			countryCode: rec.Country.ISOCode,
		}
		if country, ok := rec.Country.Names["en"]; ok {
			returnVal.country = country
		}
		return &returnVal, nil
	}
}

// CreateCountryDBLookupIso88591 CreateCountryDBLookup.
func CreateCountryDBLookupIso88591(rdr *geoip2_iso88591.CountryReader) LookupGeoIPCountry {
	return func(ip net.IP) (*GeoIPCountryResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPCountryResult{
			country:     Unknown,
			countryCode: rec.Country.ISOCode,
		}
		if country, ok := rec.Country.Names["en"]; ok {
			returnVal.country = country
		}
		return &returnVal, nil
	}
}

// NewLookupCountry returns the shared country lookup singleton, creating it on the first call.
func NewLookupCountry(dbPath, name string, iso88591 bool) (LookupGeoIPCountry, error) {
	lookupCountryMu.Lock()
	defer lookupCountryMu.Unlock()

	if lookupCountryInstance != nil {
		return lookupCountryInstance, nil
	}

	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("country DB not found: db=%s, name=%s, err=%w", dbPath, name, err)
	}

	if iso88591 {
		rdr, err := geoip2_iso88591.NewCountryReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("country lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupCountryInstance = CreateCountryDBLookupIso88591(rdr)
	} else {
		rdr, err := geoip2.NewCountryReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("country lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupCountryInstance = CreateCountryDBLookup(rdr)
	}
	return lookupCountryInstance, nil
}

// ResetLookupCountry clears the singleton for testing.
func ResetLookupCountry() {
	lookupCountryMu.Lock()
	defer lookupCountryMu.Unlock()
	lookupCountryInstance = nil
}
