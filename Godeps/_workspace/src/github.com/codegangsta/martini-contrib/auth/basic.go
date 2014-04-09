package auth

import (
	"encoding/base64"
	"net/http"
)

// Basic returns a Handler that authenticates via Basic Auth. Writes a http.StatusUnauthorized
// if authentication fails
func Basic(username string, password string) http.HandlerFunc {
	var siteAuth = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return func(res http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		if !SecureCompare(auth, "Basic "+siteAuth) {
			res.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
			http.Error(res, "Not Authorized", http.StatusUnauthorized)
		}
	}
}
