package httptoo

import (
	"net/http"
	"net/url"
)

// Deep copies a URL.
func CopyURL(u *url.URL) (ret *url.URL) {
	ret = new(url.URL)
	*ret = *u
	if u.User != nil {
		ret.User = new(url.Userinfo)
		*ret.User = *u.User
	}
	return
}

// Reconstructs the URL that would have produced the given Request.
// Request.URLs are not fully populated in http.Server handlers.
func RequestedURL(r *http.Request) (ret *url.URL) {
	ret = CopyURL(r.URL)
	ret.Host = r.Host
	ret.Scheme = OriginatingProtocol(r)
	return
}
