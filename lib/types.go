// Package lib package contains traefikgeoip implementations.
package lib

import "net/http"

// StripHeaders removes all X-GeoIP-* and X-Traffic-* headers from an inbound request
// to prevent clients from spoofing middleware-owned headers.
func StripHeaders(req *http.Request) {
	req.Header.Del(ContinentHeader)
	req.Header.Del(ContinentCodeHeader)
	req.Header.Del(CountryHeader)
	req.Header.Del(CountryCodeHeader)
	req.Header.Del(RegionHeader)
	req.Header.Del(RegionCodeHeader)
	req.Header.Del(CityHeader)
	req.Header.Del(PostalCodeHeader)
	req.Header.Del(LatitudeHeader)
	req.Header.Del(LongitudeHeader)
	req.Header.Del(AccuracyRadiusHeader)
	req.Header.Del(GeohashHeader)
	req.Header.Del(ASNSystemNumberHeader)
	req.Header.Del(ASNOrganizationHeader)
	req.Header.Del(IPAddressHeader)
	req.Header.Del(TrafficTypeHeader)
	req.Header.Del(TrafficDatacenterHeader)
	req.Header.Del(TrafficVPNHeader)
	req.Header.Del(TrafficTorHeader)
	req.Header.Del(TrafficAIBotHeader)
}

// TraefikGeoIPNotFound is a middleware that handles the case when no GeoIP DB is configured.
type TraefikGeoIPNotFound struct {
	Next       http.Handler
	Name       string
	Options    Options
	Classifier *Classifier
}

func (mw *TraefikGeoIPNotFound) ServeHTTP(reqWr http.ResponseWriter, req *http.Request) {
	StripHeaders(req)
	ipStr := getClientIP(req, mw.Options)
	req.Header.Set(IPAddressHeader, ipStr)
	mw.Classifier.Classify(req, ipStr, "")
	mw.Next.ServeHTTP(reqWr, req)
}

// Options the plugin options.
type Options struct {
	PreferXForwardedForHeader bool
	IPHeader                  string `json:"ipHeader,omitempty"`
	FailInError               bool   `json:"failInError,omitempty"`
	Debug                     bool   `json:"debug,omitempty"`
	LightMode                 bool   `json:"lightMode,omitempty"`
	Iso88591                  bool   `json:"iso88591,omitempty"`
}

// Config the plugin configuration.
type Config struct {
	CityDBPath                string `json:"cityDbPath,omitempty"`
	AsnDBPath                 string `json:"asnDbPath,omitempty"`
	CountryDBPath             string `json:"countryDbPath,omitempty"`
	PreferXForwardedForHeader bool
	IPHeader                  string `json:"ipHeader,omitempty"`
	FailInError               bool   `json:"failInError,omitempty"`
	Debug                     bool   `json:"debug,omitempty"`
	LightMode                 bool   `json:"lightMode,omitempty"`
	Iso88591                  bool   `json:"iso88591,omitempty"`
	DatacenterFile            string `json:"datacenterFile,omitempty"`
	VPNFile                   string `json:"vpnFile,omitempty"`
	TorFile                   string `json:"torFile,omitempty"`
	AIBotFile                 string `json:"aiBotFile,omitempty"`
	RefreshSeconds            int    `json:"refreshSeconds,omitempty"`
}

// ConfigToOptions converts the plugin configuration to plugin options.
func ConfigToOptions(config *Config) Options {
	return Options{
		PreferXForwardedForHeader: config.PreferXForwardedForHeader,
		IPHeader:                  config.IPHeader,
		FailInError:               config.FailInError,
		Debug:                     config.Debug,
		LightMode:                 config.LightMode,
		Iso88591:                  config.Iso88591,
	}
}

// DefaultDBPath default GeoIP2 database path.
const DefaultDBPath = "GeoLite2-City.mmdb"

const (
	// Unknown constant for undefined data.
	Unknown = "XX"
	// ContinentHeader continent header name.
	ContinentHeader = "X-GeoIP-Continent"
	// ContinentCodeHeader continent code header name.
	ContinentCodeHeader = "X-GeoIP-Continent-Code"
	// CountryHeader country header name.
	CountryHeader = "X-GeoIP-Country"
	// CountryCodeHeader country code header name.
	CountryCodeHeader = "X-GeoIP-Country-Code"
	// RegionHeader region header name.
	RegionHeader = "X-GeoIP-Region"
	// RegionCodeHeader region code header name.
	RegionCodeHeader = "X-GeoIP-Region-Code"
	// CityHeader city header name.
	CityHeader = "X-GeoIP-City"
	// PostalCodeHeader postal code header name.
	PostalCodeHeader = "X-GeoIP-Postal-Code"

	// LatitudeHeader latitude header name.
	LatitudeHeader = "X-GeoIP-Latitude"
	// LongitudeHeader longitude header name.
	LongitudeHeader = "X-GeoIP-Longitude"
	// AccuracyRadiusHeader coord accuracy radius header name.
	AccuracyRadiusHeader = "X-GeoIP-Accuracy-Radius"
	// GeohashHeader geohash header name.
	GeohashHeader = "X-GeoIP-Geohash"

	// ASNSystemNumberHeader asn system number header name.
	ASNSystemNumberHeader = "X-GeoIP-ASN-System-Number"
	// ASNOrganizationHeader asn system organization header name.
	ASNOrganizationHeader = "X-GeoIP-ASN-Organization"

	// IPAddressHeader client IP header name.
	IPAddressHeader = "X-GeoIP-IPAddress"

	// TrafficTypeHeader traffic classification header.
	TrafficTypeHeader = "X-Traffic-Type"
	// TrafficDatacenterHeader datacenter detection header.
	TrafficDatacenterHeader = "X-Traffic-Datacenter"
	// TrafficVPNHeader VPN detection header.
	TrafficVPNHeader = "X-Traffic-VPN"
	// TrafficTorHeader Tor detection header.
	TrafficTorHeader = "X-Traffic-Tor"
	// TrafficAIBotHeader AI bot detection header.
	TrafficAIBotHeader = "X-Traffic-AI-Bot"
)
