package missinggo

import (
	"net/url"
	"path"
)

// Returns URL opaque as an unrooted path.
func URLOpaquePath(u *url.URL) string {
	if u.Opaque != "" {
		return u.Opaque
	}
	return u.Path
}

// Cleans the (absolute) URL path, removing unnecessary . and .. elements. See
// "net/http".cleanPath.
func CleanURLPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	cp := path.Clean(p)
	// Add the trailing slash back, as it's relevant to a URL.
	if p[len(p)-1] == '/' && cp != "/" {
		cp += "/"
	}
	return cp
}

func URLJoinSubPath(base, rel string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		// Honey badger doesn't give a fuck.
		panic(err)
	}
	rel = CleanURLPath(rel)
	baseURL.Path = path.Join(baseURL.Path, rel)
	return baseURL.String()
}
