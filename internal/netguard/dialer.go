package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

type Dialer struct {
	Resolver     Resolver
	AllowPrivate bool
	Dialer       net.Dialer
	Dial         DialFunc
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split target address: %w", err)
	}
	resolver := d.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dial := d.Dial
	if dial == nil {
		dial = d.Dialer.DialContext
	}

	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve target %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("target %q resolved to no addresses", host)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].IP.String() < addresses[j].IP.String()
	})

	var blocked []string
	var dialErrors []error
	for _, address := range addresses {
		ip := address.IP
		if !d.AllowPrivate && IsBlocked(ip) {
			blocked = append(blocked, ip.String())
			continue
		}

		connection, dialErr := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, fmt.Errorf("dial %s: %w", ip.String(), dialErr))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if len(dialErrors) > 0 {
		return nil, fmt.Errorf("all permitted target addresses failed: %w", errors.Join(dialErrors...))
	}
	return nil, fmt.Errorf("target resolves only to blocked addresses: %s", strings.Join(blocked, ", "))
}

func IsBlocked(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
