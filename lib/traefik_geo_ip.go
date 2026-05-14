package lib

import (
	"net/http"
)

// TraefikGeoIP is a middleware that put ip in header.
type TraefikGeoIP struct {
	Next       http.Handler
	Name       string
	Options    Options
	Classifier *Classifier
}

func (mw *TraefikGeoIP) ServeHTTP(reqWr http.ResponseWriter, req *http.Request) {
	StripHeaders(req)
	ipStr := getClientIP(req, mw.Options)
	req.Header.Set(IPAddressHeader, ipStr)
	mw.Classifier.Classify(req, ipStr, "")
	mw.Next.ServeHTTP(reqWr, req)
}
