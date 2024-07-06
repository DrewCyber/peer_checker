package main

import (
	"errors"
	"net"
	"testing"
)

func TestResolve(t *testing.T) {
	// Test case 1: Valid name
	name := "example.com"
	expectedIP := []net.IP{net.ParseIP("93.184.216.34")}
	resolver := func(name string) ([]net.IP, error) {
		return expectedIP, nil
	}

	ip, err := resolve(name, resolver)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if ip != expectedIP[0].String() {
		t.Errorf("Expected IP: %s, but got: %s", expectedIP, ip)
	}

	// Test case 2: Resolver error
	expectedErr := "Resolver error"
	resolver = func(name string) ([]net.IP, error) {
		return nil, errors.New(expectedErr)
	}

	_, err = resolve(name, resolver)
	if err == nil {
		t.Errorf("Expected error, but got nil")
	}

	if err.Error() != expectedErr {
		t.Errorf("Expected error: %s, but got: %v", expectedErr, err)
	}

	// Test case 3: IPv6 address
	expectedIP = []net.IP{net.ParseIP("2001:4860:4860::8888")}
	resolver = func(name string) ([]net.IP, error) {
		return nil, nil
	}
	ip, err = resolve("[2001:4860:4860::8888]", resolver)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ip != expectedIP[0].String() {
		t.Errorf("Expected IP: %s, but got: %s", expectedIP, ip)
	}
}
