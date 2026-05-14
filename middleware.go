// Package traefikclassifier is a Traefik plugin for GeoIP lookups and traffic classification.
package traefikclassifier

import (
	"context"
	"log"
	"net/http"
	"os"

	lib "github.com/WebConcern/traefikclassifier/lib"
)

// CreateConfig creates the default plugin configuration.
func CreateConfig() *lib.Config {
	return &lib.Config{}
}

// New creates a new traefik-classifier plugin instance.
func New(_ context.Context, next http.Handler, cfg *lib.Config, name string) (http.Handler, error) {
	lookupCity, lookupCountry, lookupAsn, err := factoryLookups(cfg, name)

	classifier := lib.NewClassifier(cfg)

	if err != nil {
		if cfg.FailInError {
			log.Fatalf("%s", err.Error())
		}

		stderrLogger := log.New(os.Stderr, "ERROR: ", log.LstdFlags|log.Lshortfile)
		stderrLogger.Printf("%s. Only processing IpHeader.", err.Error())
		return &lib.TraefikGeoIP{
			Next:       next,
			Name:       name,
			Options:    lib.ConfigToOptions(cfg),
			Classifier: classifier,
		}, nil
	}

	opts := lib.ConfigToOptions(cfg)

	switch {
	case cfg.LightMode && lookupCity != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCityAsnLightMode{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
			LookupAsn:  lookupAsn,
			LookupCity: lookupCity,
		}, nil

	case lookupCity != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCityAsn{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
			LookupAsn:  lookupAsn,
			LookupCity: lookupCity,
		}, nil
	case cfg.LightMode && lookupCity != nil:
		return &lib.TraefikGeoIPCityLightMode{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
			LookupCity: lookupCity,
		}, nil
	case lookupCity != nil:
		return &lib.TraefikGeoIPCity{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
			LookupCity: lookupCity,
		}, nil
	case lookupCountry != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCountryAsn{
			Next:          next,
			Name:          name,
			Options:       opts,
			Classifier:    classifier,
			LookupAsn:     lookupAsn,
			LookupCountry: lookupCountry,
		}, nil
	case lookupCountry != nil:
		return &lib.TraefikGeoIPCountry{
			Next:          next,
			Name:          name,
			Options:       opts,
			Classifier:    classifier,
			LookupCountry: lookupCountry,
		}, nil
	case lookupAsn != nil:
		return &lib.TraefikGeoIPAsn{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
			LookupAsn:  lookupAsn,
		}, nil
	default:
		return &lib.TraefikGeoIPNotFound{
			Next:       next,
			Name:       name,
			Options:    opts,
			Classifier: classifier,
		}, nil
	}
}

func factoryLookups(cfg *lib.Config, name string) (lib.LookupGeoIPCity, lib.LookupGeoIPCountry, lib.LookupGeoIPAsn, error) {
	var lookupCity lib.LookupGeoIPCity
	var lookupCountry lib.LookupGeoIPCountry
	var lookupAsn lib.LookupGeoIPAsn

	if cfg.CityDBPath != "" {
		var err error
		lookupCity, err = lib.NewLookupCity(cfg.CityDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	} else if cfg.CountryDBPath != "" {
		var err error
		lookupCountry, err = lib.NewLookupCountry(cfg.CountryDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if cfg.AsnDBPath != "" {
		var err error
		lookupAsn, err = lib.NewLookupAsn(cfg.AsnDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return lookupCity, lookupCountry, lookupAsn, nil
}
