// Package geo does best-effort IP-to-country resolution from a MaxMind GeoLite2 database - optional, so with no DB path configured the resolver is a no-op and dev, CI and deployments without a GeoLite2 file work unchanged, with geo simply absent from profiles
package geo

import (
	"net/netip"

	"github.com/oschwald/geoip2-golang/v2"
)

// Wraps an opened GeoLite2 reader, Country or City, since both expose Country - a nil or empty Resolver is a valid no-op
type Resolver struct {
	db *geoip2.Reader
}

// Loads a GeoLite2 .mmdb - an empty path yields a no-op resolver with no error, so callers needn't special-case the unconfigured case
func Open(path string) (*Resolver, error) {
	if path == "" {
		return &Resolver{}, nil
	}
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &Resolver{db: db}, nil
}

func (r *Resolver) Enabled() bool { return r != nil && r.db != nil }

// The English country name for an IP, or "" when geo is disabled, the IP is unparseable or private, or the lookup misses
func (r *Resolver) Country(ip string) string {
	if !r.Enabled() || ip == "" {
		return ""
	}
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	rec, err := r.db.Country(parsed)
	if err != nil {
		return ""
	}
	return rec.Country.Names.English
}

func (r *Resolver) Close() error {
	if r != nil && r.db != nil {
		return r.db.Close()
	}
	return nil
}
