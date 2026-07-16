package netguard

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type staticResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses...), r.err
}

func TestDialerBlocksPrivateAddressesByDefault(t *testing.T) {
	dialer := &Dialer{Resolver: staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}}
	_, err := dialer.DialContext(context.Background(), "tcp", "example.test:443")
	if err == nil || !strings.Contains(err.Error(), "blocked addresses") {
		t.Fatalf("expected blocked address error, got %v", err)
	}
}

func TestDialerTriesAllPermittedAddresses(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	attempts := make([]string, 0, 2)
	dialer := &Dialer{
		Resolver: staticResolver{addresses: []net.IPAddr{
			{IP: net.ParseIP("203.0.113.20")},
			{IP: net.ParseIP("203.0.113.10")},
		}},
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			attempts = append(attempts, address)
			if strings.HasPrefix(address, "203.0.113.10") {
				return nil, errors.New("unreachable")
			}
			return client, nil
		},
	}

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.test:443")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	if len(attempts) != 2 || !strings.HasPrefix(attempts[0], "203.0.113.10") || !strings.HasPrefix(attempts[1], "203.0.113.20") {
		t.Fatalf("unexpected dial attempts: %#v", attempts)
	}
}

func TestDialerPreservesResolverFailure(t *testing.T) {
	dialer := &Dialer{Resolver: staticResolver{err: errors.New("resolver unavailable")}}
	_, err := dialer.DialContext(context.Background(), "tcp", "example.test:443")
	if err == nil || !strings.Contains(err.Error(), "resolve target") {
		t.Fatalf("expected resolver error, got %v", err)
	}
}

func TestIsBlocked(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "224.0.0.1", "0.0.0.0", "::1"} {
		if !IsBlocked(net.ParseIP(address)) {
			t.Fatalf("expected %s to be blocked", address)
		}
	}
	if IsBlocked(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address was blocked")
	}
}
