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

// Traefik calls New once per router using the middleware; all of them share one database.
//
//nolint:gochecknoglobals // process-wide singleton shared by all middleware instances
var (
	lookupAsnMu       sync.Mutex
	lookupAsnInstance LookupGeoIPAsn

	// asnDB is the currently loaded database; it is swapped when the file changes.
	asnDBMu sync.RWMutex
	asnDB   LookupGeoIPAsn
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

// NewLookupAsn returns the shared asn lookup singleton, creating it on the first call.
// The database is reloaded by RefreshFiles when the file changes; a replacement that
// fails to load is ignored and the previous database stays in use.
func NewLookupAsn(dbPath, name string, iso88591 bool) (LookupGeoIPAsn, error) {
	lookupAsnMu.Lock()
	defer lookupAsnMu.Unlock()

	if lookupAsnInstance != nil {
		return lookupAsnInstance, nil
	}

	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("asn DB not found: db=%s, name=%s, err=%w", dbPath, name, err)
	}

	stamp := statFile(dbPath)
	lookup, err := openAsnDB(dbPath, iso88591)
	if err != nil {
		return nil, fmt.Errorf("asn lookup DB is not initialized: db=%s, name=%s, err=%w", dbPath, name, err)
	}
	setAsnDB(lookup)
	watchFile("GeoIP asn DB", dbPath, stamp, func(path string) error {
		lookup, err := openAsnDB(path, iso88591)
		if err != nil {
			return err
		}
		setAsnDB(lookup)
		return nil
	})

	lookupAsnInstance = func(ip net.IP) (*GeoIPAsnResult, error) {
		asnDBMu.RLock()
		lookup := asnDB
		asnDBMu.RUnlock()
		return lookup(ip)
	}
	return lookupAsnInstance, nil
}

func openAsnDB(dbPath string, iso88591 bool) (LookupGeoIPAsn, error) {
	if iso88591 {
		rdr, err := geoip2_iso88591.NewASNReaderFromFile(dbPath)
		if err != nil {
			return nil, err
		}
		return CreateAsnDBLookupIso88591(rdr), nil
	}
	rdr, err := geoip2.NewASNReaderFromFile(dbPath)
	if err != nil {
		return nil, err
	}
	return CreateAsnDBLookup(rdr), nil
}

func setAsnDB(lookup LookupGeoIPAsn) {
	asnDBMu.Lock()
	asnDB = lookup
	asnDBMu.Unlock()
}

// ResetLookupAsn clears the singleton for testing.
func ResetLookupAsn() {
	lookupAsnMu.Lock()
	defer lookupAsnMu.Unlock()
	lookupAsnInstance = nil
}
