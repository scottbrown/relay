package acl

import (
	"net"
	"testing"
)

func TestNew_Empty(t *testing.T) {
	list, err := New("")
	if err != nil {
		t.Fatalf("New with empty string should succeed: %v", err)
	}

	if len(list.nets) != 0 {
		t.Errorf("expected empty nets slice, got %d items", len(list.nets))
	}
}

func TestNew_Whitespace(t *testing.T) {
	list, err := New("   \t\n  ")
	if err != nil {
		t.Fatalf("New with whitespace should succeed: %v", err)
	}

	if len(list.nets) != 0 {
		t.Errorf("expected empty nets slice, got %d items", len(list.nets))
	}
}

func TestNew_SingleCIDR(t *testing.T) {
	list, err := New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("New with valid CIDR should succeed: %v", err)
	}

	if len(list.nets) != 1 {
		t.Errorf("expected 1 net, got %d", len(list.nets))
	}

	expected := "192.168.1.0/24"
	if list.nets[0].String() != expected {
		t.Errorf("expected CIDR %q, got %q", expected, list.nets[0].String())
	}
}

func TestNew_MultipleCIDRs(t *testing.T) {
	cidrs := "192.168.1.0/24,10.0.0.0/8,172.16.0.0/12"
	list, err := New(cidrs)
	if err != nil {
		t.Fatalf("New with multiple CIDRs should succeed: %v", err)
	}

	if len(list.nets) != 3 {
		t.Errorf("expected 3 nets, got %d", len(list.nets))
	}

	expectedCIDRs := []string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.0/12"}
	for i, expected := range expectedCIDRs {
		if list.nets[i].String() != expected {
			t.Errorf("expected CIDR %q at index %d, got %q", expected, i, list.nets[i].String())
		}
	}
}

func TestNew_WithSpaces(t *testing.T) {
	cidrs := " 192.168.1.0/24 , 10.0.0.0/8 ,  172.16.0.0/12  "
	list, err := New(cidrs)
	if err != nil {
		t.Fatalf("New with spaces around CIDRs should succeed: %v", err)
	}

	if len(list.nets) != 3 {
		t.Errorf("expected 3 nets, got %d", len(list.nets))
	}
}

func TestNew_InvalidCIDR(t *testing.T) {
	invalidCIDRs := []string{
		"invalid-cidr",
		"192.168.1.256/24",
		"192.168.1.1/33",
		"192.168.1.0/24,invalid,10.0.0.0/8",
	}

	for _, cidr := range invalidCIDRs {
		_, err := New(cidr)
		if err == nil {
			t.Errorf("New with invalid CIDR %q should fail", cidr)
		}
	}
}

func TestAllows_EmptyList(t *testing.T) {
	list, _ := New("")

	testIPs := []string{
		"192.168.1.1",
		"10.0.0.1",
		"8.8.8.8",
		"::1",
	}

	for _, ipStr := range testIPs {
		ip := net.ParseIP(ipStr)
		if !list.Allows(ip) {
			t.Errorf("empty ACL should allow all IPs, but rejected %s", ipStr)
		}
	}
}

func TestAllows_SingleCIDR(t *testing.T) {
	list, err := New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	allowedIPs := []string{
		"192.168.1.1",
		"192.168.1.100",
		"192.168.1.254",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if !list.Allows(ip) {
			t.Errorf("ACL should allow %s", ipStr)
		}
	}

	rejectedIPs := []string{
		"192.168.2.1",
		"10.0.0.1",
		"8.8.8.8",
	}

	for _, ipStr := range rejectedIPs {
		ip := net.ParseIP(ipStr)
		if list.Allows(ip) {
			t.Errorf("ACL should reject %s", ipStr)
		}
	}
}

func TestAllows_MultipleCIDRs(t *testing.T) {
	list, err := New("192.168.1.0/24,10.0.0.0/8")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	allowedIPs := []string{
		"192.168.1.1",
		"192.168.1.254",
		"10.0.0.1",
		"10.255.255.254",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if !list.Allows(ip) {
			t.Errorf("ACL should allow %s", ipStr)
		}
	}

	rejectedIPs := []string{
		"192.168.2.1",
		"172.16.0.1",
		"8.8.8.8",
	}

	for _, ipStr := range rejectedIPs {
		ip := net.ParseIP(ipStr)
		if list.Allows(ip) {
			t.Errorf("ACL should reject %s", ipStr)
		}
	}
}

func TestAllows_IPv6(t *testing.T) {
	list, err := New("2001:db8::/32")
	if err != nil {
		t.Fatalf("failed to create IPv6 ACL: %v", err)
	}

	allowedIPs := []string{
		"2001:db8::1",
		"2001:db8:1:2::3",
		"2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if !list.Allows(ip) {
			t.Errorf("ACL should allow IPv6 %s", ipStr)
		}
	}

	rejectedIPs := []string{
		"2001:db9::1",
		"::1",
		"2002:db8::1",
	}

	for _, ipStr := range rejectedIPs {
		ip := net.ParseIP(ipStr)
		if list.Allows(ip) {
			t.Errorf("ACL should reject IPv6 %s", ipStr)
		}
	}
}

func TestAllows_MixedIPv4IPv6(t *testing.T) {
	list, err := New("192.168.1.0/24,2001:db8::/32")
	if err != nil {
		t.Fatalf("failed to create mixed ACL: %v", err)
	}

	allowedIPs := []string{
		"192.168.1.1",
		"2001:db8::1",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if !list.Allows(ip) {
			t.Errorf("ACL should allow %s", ipStr)
		}
	}

	rejectedIPs := []string{
		"10.0.0.1",
		"2001:db9::1",
	}

	for _, ipStr := range rejectedIPs {
		ip := net.ParseIP(ipStr)
		if list.Allows(ip) {
			t.Errorf("ACL should reject %s", ipStr)
		}
	}
}
