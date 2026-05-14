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
	lookupAsnMu       sync.Mutex
	lookupAsnInstance LookupGeoIPAsn
)

// GeoIPAsnResult in memory, this should have between 126 and 180 bytes. On average, consider 150 bytes.
type GeoIPAsnResult struct {
	number       string
	organization string
}

// LookupGeoIPAsn LookupGeoIP.
type LookupGeoIPAsn func(ip net.IP) (*GeoIPAsnResult, error)

// CreateAsnDBLookup CreateCountryDBLookup.
func CreateAsnDBLookup(rdr *geoip2.ASNReader) LookupGeoIPAsn {
	return func(ip net.IP) (*GeoIPAsnResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPAsnResult{
			number:       strconv.Itoa(int(rec.AutonomousSystemNumber)),
			organization: rec.AutonomousSystemOrganization,
		}
		return &returnVal, nil
	}
}

// CreateAsnDBLookupIso88591 CreateCountryDBLookup.
func CreateAsnDBLookupIso88591(rdr *geoip2_iso88591.ASNReader) LookupGeoIPAsn {
	return func(ip net.IP) (*GeoIPAsnResult, error) {
		rec, err := rdr.Lookup(ip)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		returnVal := GeoIPAsnResult{
			number:       strconv.Itoa(int(rec.AutonomousSystemNumber)),
			organization: rec.AutonomousSystemOrganization,
		}
		return &returnVal, nil
	}
}

// NewLookupAsn returns the shared ASN lookup singleton, creating it on the first call.
func NewLookupAsn(dbPath, name string, iso88591 bool) (LookupGeoIPAsn, error) {
	lookupAsnMu.Lock()
	defer lookupAsnMu.Unlock()

	if lookupAsnInstance != nil {
		return lookupAsnInstance, nil
	}

	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("asn DB not found: db=%s, name=%s, err=%w", dbPath, name, err)
	}

	if iso88591 {
		rdr, err := geoip2_iso88591.NewASNReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("asn lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupAsnInstance = CreateAsnDBLookupIso88591(rdr)
	} else {
		rdr, err := geoip2.NewASNReaderFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("asn lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
		}
		lookupAsnInstance = CreateAsnDBLookup(rdr)
	}
	return lookupAsnInstance, nil
}

// ResetLookupAsn clears the singleton for testing.
func ResetLookupAsn() {
	lookupAsnMu.Lock()
	defer lookupAsnMu.Unlock()
	lookupAsnInstance = nil
}
