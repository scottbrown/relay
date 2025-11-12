// Package acl provides CIDR-based IP access control for incoming connections.
package acl

import (
	"net"
	"strings"
)

// List represents a collection of CIDR networks for access control.
// An empty list allows all connections.
type List struct {
	nets []*net.IPNet
}

// New creates a new List from a comma-separated string of CIDR blocks.
// An empty string results in a list that allows all IPs.
// Returns an error if any CIDR block cannot be parsed.
func New(cidrs string) (*List, error) {
	if strings.TrimSpace(cidrs) == "" {
		return &List{}, nil
	}

	var nets []*net.IPNet
	for _, part := range strings.Split(cidrs, ",") {
		_, n, err := net.ParseCIDR(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}

	return &List{nets: nets}, nil
}

// Allows checks if the given IP address is allowed by this ACL.
// If no networks are configured, all IPs are allowed.
// Returns true if the IP matches any configured CIDR block.
func (l *List) Allows(ip net.IP) bool {
	if len(l.nets) == 0 {
		return true
	}

	for _, n := range l.nets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}
