package httptoo

import (
	"crypto/tls"
	"net/http"
)

// Returns the http.Client's TLS Config, traversing and generating any
// defaults along the way to get it.
func ClientTLSConfig(cl *http.Client) *tls.Config {
	if cl.Transport == nil {
		cl.Transport = http.DefaultTransport
	}
	tr := cl.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	}
	return tr.TLSClientConfig
}
