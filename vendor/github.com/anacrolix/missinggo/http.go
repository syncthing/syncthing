package missinggo

import "strconv"

// Performs quoted-string from http://www.w3.org/Protocols/rfc2616/rfc2616-sec2.html
func HTTPQuotedString(s string) string {
	return strconv.Quote(s)
}
