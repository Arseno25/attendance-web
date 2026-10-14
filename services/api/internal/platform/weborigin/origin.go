package weborigin

import (
	"net/url"
	"strconv"
)

func Allowed(origin, configured string, allowLocal bool) bool {
	if origin == configured {
		return true
	}
	if !allowLocal {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "localhost" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(parsed.Port())
	return err == nil && port >= 3001 && port <= 3010
}
