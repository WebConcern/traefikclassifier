package lib

import (
	"log"
	"net"
	"net/http"
)

// TraefikGeoIPCountryAsn is a middleware that looks up the city of the client IP address from the GeoIP2 database.
type TraefikGeoIPCountryAsn struct {
	Next          http.Handler
	Name          string
	Options       Options
	Classifier    *Classifier
	LookupAsn     LookupGeoIPAsn
	LookupCountry LookupGeoIPCountry
}

func (mw *TraefikGeoIPCountryAsn) ServeHTTP(reqWr http.ResponseWriter, req *http.Request) {
	StripHeaders(req)
	ipStr := getClientIP(req, mw.Options)
	req.Header.Set(IPAddressHeader, ipStr)
	res, err := mw.LookupCountry(net.ParseIP(ipStr))
	if err != nil {
		if mw.Options.Debug {
			log.Printf("[geoip2] Unable to find Country: ip=%s, err=%v", ipStr, err)
		}
		req.Header.Set(CountryHeader, Unknown)
		req.Header.Set(CountryCodeHeader, Unknown)
	} else {
		req.Header.Set(CountryHeader, res.country)
		req.Header.Set(CountryCodeHeader, res.countryCode)
	}
	asnNumber := Unknown
	resAsn, err := mw.LookupAsn(net.ParseIP(ipStr))
	if err != nil {
		if mw.Options.Debug {
			log.Printf("[geoip2] Unable to find ASN: ip=%s, err=%v", ipStr, err)
		}
		req.Header.Set(ASNSystemNumberHeader, Unknown)
		req.Header.Set(ASNOrganizationHeader, Unknown)
	} else {
		asnNumber = resAsn.number
		req.Header.Set(ASNSystemNumberHeader, resAsn.number)
		req.Header.Set(ASNOrganizationHeader, resAsn.organization)
	}

	mw.Classifier.Classify(req, ipStr, asnNumber)
	mw.Next.ServeHTTP(reqWr, req)
}
