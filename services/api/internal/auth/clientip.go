package auth

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// Only a configured proxy may supply a chain; walk from the peer toward the client.
func (s *Service) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	peer = peer.Unmap()
	trusted := func(ip netip.Addr) bool {
		return slices.ContainsFunc(s.trustedProxies, func(prefix netip.Prefix) bool { return prefix.Contains(ip) })
	}
	if !trusted(peer) {
		return peer.String()
	}
	header := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if len(header) > 2048 {
		return peer.String()
	}
	hops := strings.Split(header, ",")
	if len(hops) > 20 {
		return peer.String()
	}
	client := peer
	for i := len(hops) - 1; i >= 0 && trusted(client); i-- {
		next, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return peer.String()
		}
		client = next.Unmap()
	}
	return client.String()
}
