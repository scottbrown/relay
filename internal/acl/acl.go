package acl

import (
	"net"
	"strings"
)

// List represents a collection of CIDR networks for access control
type List struct {
	nets []*net.IPNet
}

// New creates a new ACL from a comma-separated string of CIDR blocks
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

// Allows checks if the given IP address is allowed by this ACL
// If no networks are configured, all IPs are allowed
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
