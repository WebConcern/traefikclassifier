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

	// countryDB is the currently loaded database; it is swapped when the file changes.
	countryDBMu sync.RWMutex
	countryDB   LookupGeoIPCountry
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
// The database is reloaded by RefreshFiles when the file changes; a replacement that
// fails to load is ignored and the previous database stays in use.
func NewLookupCountry(dbPath, name string, iso88591 bool) (LookupGeoIPCountry, error) {
	lookupCountryMu.Lock()
	defer lookupCountryMu.Unlock()

	if lookupCountryInstance != nil {
		return lookupCountryInstance, nil
	}

	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("country DB not found: db=%s, name=%s, err=%w", dbPath, name, err)
	}

	stamp := statFile(dbPath)
	lookup, err := openCountryDB(dbPath, iso88591)
	if err != nil {
		return nil, fmt.Errorf("country lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
	}
	setCountryDB(lookup)
	watchFile("GeoIP country DB", dbPath, stamp, func(path string) error {
		lookup, err := openCountryDB(path, iso88591)
		if err != nil {
			return err
		}
		setCountryDB(lookup)
		return nil
	})

	lookupCountryInstance = func(ip net.IP) (*GeoIPCountryResult, error) {
		countryDBMu.RLock()
		lookup := countryDB
		countryDBMu.RUnlock()
		return lookup(ip)
	}
	return lookupCountryInstance, nil
}

func openCountryDB(dbPath string, iso88591 bool) (LookupGeoIPCountry, error) {
	if iso88591 {
		rdr, err := geoip2_iso88591.NewCountryReaderFromFile(dbPath)
		if err != nil {
			return nil, err
		}
		return CreateCountryDBLookupIso88591(rdr), nil
	}
	rdr, err := geoip2.NewCountryReaderFromFile(dbPath)
	if err != nil {
		return nil, err
	}
	return CreateCountryDBLookup(rdr), nil
}

func setCountryDB(lookup LookupGeoIPCountry) {
	countryDBMu.Lock()
	countryDB = lookup
	countryDBMu.Unlock()
}

// ResetLookupCountry clears the singleton for testing.
func ResetLookupCountry() {
	lookupCountryMu.Lock()
	defer lookupCountryMu.Unlock()
	lookupCountryInstance = nil
}
