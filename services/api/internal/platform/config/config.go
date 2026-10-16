package config

import (
	"attendance.local/api/internal/platform/aiclient"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Address, DatabaseURL, WebOrigin, AIBaseURL, AIServiceToken string
	AllowLocalWebPortRange                                     bool
	TrustedProxies                                             []netip.Prefix
}

func Load() (Config, error) {
	c := Config{Address: os.Getenv("API_ADDRESS"), DatabaseURL: os.Getenv("DATABASE_URL"), WebOrigin: os.Getenv("WEB_ORIGIN"), AIBaseURL: os.Getenv("AI_BASE_URL"), AIServiceToken: os.Getenv("AI_SERVICE_TOKEN")}
	if value := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS")); value != "" {
		for _, raw := range strings.Split(value, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
			if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
				return c, errors.New("TRUSTED_PROXY_CIDRS must contain explicit IPv4/IPv6 proxy CIDRs, not a wildcard")
			}
			c.TrustedProxies = append(c.TrustedProxies, prefix.Masked())
		}
	}
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(c.Address)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || n < 1 || n > 65535 {
		return c, errors.New("API_ADDRESS must be a host:port address")
	}
	db, err := url.Parse(c.DatabaseURL)
	if err != nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Host == "" {
		return c, errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	origin, err := url.Parse(c.WebOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return c, errors.New("WEB_ORIGIN must be an exact http(s) origin")
	}
	if origin.Scheme == "http" && origin.Hostname() != "localhost" && !net.ParseIP(origin.Hostname()).IsLoopback() {
		return c, errors.New("public WEB_ORIGIN requires HTTPS")
	}
	c.AllowLocalWebPortRange = os.Getenv("ALLOW_LOCAL_WEB_PORT_RANGE") == "1"
	if c.AllowLocalWebPortRange && (origin.Scheme != "http" || origin.Hostname() != "localhost") {
		return c, errors.New("local web port allowlist requires an HTTP localhost origin")
	}
	if c.AllowLocalWebPortRange && origin.Port() != "" {
		webPort, err := strconv.Atoi(origin.Port())
		if err != nil || webPort < 3001 || webPort > 3010 {
			return c, errors.New("local web port allowlist requires a port from 3001 through 3010")
		}
	}

	if c.AIBaseURL != "" {
		if _, err := aiclient.New(c.AIBaseURL, c.AIServiceToken); err != nil {
			return c, err
		}
	}
	return c, nil
}
