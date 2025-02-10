package resolver

import (
	"context"
	"crypto/tls"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"time"

	"github.com/miekg/dns"
	"github.com/samber/lo"

	"github.com/yaling888/quirktiva/component/trie"
)

var (
	// DefaultResolver aim to resolve ip
	DefaultResolver Resolver

	// DisableIPv6 means don't resolve ipv6 host
	// default value is true
	DisableIPv6 = true

	// RemoteDnsResolve reports whether TCP/UDP handler should be remote resolve DNS
	// default value is true
	RemoteDnsResolve = true

	// DefaultHosts aim to resolve hosts
	DefaultHosts = trie.New[netip.Addr]()

	// DefaultDNSTimeout defined the default dns request timeout
	DefaultDNSTimeout = time.Second * 5

	needProxyHostIPv6 = false
)

var (
	ErrIPNotFound      = errors.New("can not find ip")
	ErrIPVersion       = errors.New("ip version error")
	ErrIPv6Disabled    = errors.New("ipv6 disabled")
	ErrECHNotFound     = errors.New("can not find ECH")
	ErrECHNotSupport   = errors.New("can not lookup ECH by unsafe client")
	ErrECHServerReject = errors.New("tls: server rejected ECH")
)

const (
	proxyServerHostKey = ipContextKey("key-lookup-proxy-server-ip")
	proxyKey           = ipContextKey("key-lookup-by-proxy")
)

const (
	typeNone uint16 = 0
	typeA    uint16 = 1
	typeAAAA uint16 = 28
)

type ipContextKey string

type Resolver interface {
	LookupIP(ctx context.Context, host string) ([]netip.Addr, error)
	LookupIPv4(ctx context.Context, host string) ([]netip.Addr, error)
	LookupIPv6(ctx context.Context, host string) ([]netip.Addr, error)
	LookupECH(ctx context.Context, host string) ([]byte, error)
	ResolveIP(host string) (ip netip.Addr, err error)
	ResolveIPv4(host string) (ip netip.Addr, err error)
	ResolveIPv6(host string) (ip netip.Addr, err error)
	ExchangeContext(ctx context.Context, m *dns.Msg) (msg *dns.Msg, source string, err error)
	ExchangeContextWithoutCache(ctx context.Context, m *dns.Msg) (msg *dns.Msg, source string, err error)
	RemoveCache(host string)
}

// LookupIP with a host, return ip list
func LookupIP(ctx context.Context, host string) ([]netip.Addr, error) {
	return LookupIPByResolver(ctx, host, DefaultResolver)
}

// LookupIPv4 with a host, return ipv4 list
func LookupIPv4(ctx context.Context, host string) ([]netip.Addr, error) {
	return lookupIPByResolverAndType(ctx, host, DefaultResolver, typeA, false)
}

// LookupIPv6 with a host, return ipv6 list
func LookupIPv6(ctx context.Context, host string) ([]netip.Addr, error) {
	return lookupIPByResolverAndType(ctx, host, DefaultResolver, typeAAAA, false)
}

// LookupIPByResolver same as ResolveIP, but with a resolver
func LookupIPByResolver(ctx context.Context, host string, r Resolver) ([]netip.Addr, error) {
	return lookupIPByResolverAndType(ctx, host, r, typeNone, false)
}

// LookupIPByProxy with a host and proxy, reports force combined ipv6 list whether the DisableIPv6 value is true
func LookupIPByProxy(ctx context.Context, host, proxy string) ([]netip.Addr, error) {
	return lookupIPByProxyAndType(ctx, host, proxy, typeNone, true)
}

// LookupIPv4ByProxy with a host and proxy, reports ipv4 list
func LookupIPv4ByProxy(ctx context.Context, host, proxy string) ([]netip.Addr, error) {
	return lookupIPByProxyAndType(ctx, host, proxy, typeA, false)
}

// LookupIPv6ByProxy with a host and proxy, reports ipv6 list whether the DisableIPv6 value is true
func LookupIPv6ByProxy(ctx context.Context, host, proxy string) ([]netip.Addr, error) {
	return lookupIPByProxyAndType(ctx, host, proxy, typeAAAA, true)
}

// LookupECHForProxyServer with a host, return ECH config list
func LookupECHForProxyServer(host string) ([]byte, error) {
	if DefaultResolver != nil {
		ctx := context.WithValue(context.Background(), proxyServerHostKey, struct{}{})
		return DefaultResolver.LookupECH(ctx, host)
	}
	return nil, ErrECHNotFound
}

// ResolveIP with a host, return ip
func ResolveIP(host string) (netip.Addr, error) {
	return resolveIPByType(host, typeNone)
}

// ResolveIPv4 with a host, return ipv4
func ResolveIPv4(host string) (netip.Addr, error) {
	return resolveIPByType(host, typeA)
}

// ResolveIPv6 with a host, return ipv6
func ResolveIPv6(host string) (netip.Addr, error) {
	return resolveIPByType(host, typeAAAA)
}

// ResolveProxyServerHost proxies server host only
func ResolveProxyServerHost(host string) (netip.Addr, error) {
	return resolveProxyServerHostByType(host, typeNone)
}

// ResolveIPv4ProxyServerHost proxies server host only
func ResolveIPv4ProxyServerHost(host string) (netip.Addr, error) {
	return resolveProxyServerHostByType(host, typeA)
}

// ResolveIPv6ProxyServerHost proxies server host only
func ResolveIPv6ProxyServerHost(host string) (netip.Addr, error) {
	return resolveProxyServerHostByType(host, typeAAAA)
}

// SetDisableIPv6 set DisableIPv6 & needProxyHostIPv6 value
func SetDisableIPv6(v bool) {
	DisableIPv6 = v
	needProxyHostIPv6 = !v
}

// RemoveCache remove cache by host
func RemoveCache(host string) {
	if DefaultResolver != nil {
		DefaultResolver.RemoveCache(host)
	}
}

// IsProxyServer reports whether the DefaultResolver should be exchanged by proxyServer DNS client
func IsProxyServer(ctx context.Context) bool {
	return ctx.Value(proxyServerHostKey) != nil
}

// IsRemote reports whether the DefaultResolver should be exchanged by remote DNS client
func IsRemote(ctx context.Context) bool {
	return ctx.Value(proxyKey) != nil
}

// GetProxy reports the proxy name used by the DNS client and whether there is a proxy
func GetProxy(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(proxyKey).(string)
	return v, ok
}

// WithProxy returns a new context with proxy name
func WithProxy(ctx context.Context, proxy string) context.Context {
	return context.WithValue(ctx, proxyKey, proxy)
}

// CopyCtxValues returns a new context with parent's values
func CopyCtxValues(parent context.Context) context.Context {
	newCtx := context.Background()
	if v, ok := parent.Value(proxyKey).(string); ok {
		newCtx = context.WithValue(newCtx, proxyKey, v)
	}
	if parent.Value(proxyServerHostKey) != nil {
		newCtx = context.WithValue(newCtx, proxyServerHostKey, struct{}{})
	}
	return newCtx
}

func SetECHConfigList(cfg *tls.Config) bool {
	ech, err := LookupECHForProxyServer(cfg.ServerName)
	if err != nil {
		if err == ErrECHNotFound || errors.Is(err, ErrECHNotSupport) {
			return false
		}
		return true
	}
	cfg.MinVersion = tls.VersionTLS13
	cfg.InsecureSkipVerify = false
	cfg.EncryptedClientHelloConfigList = ech
	cfg.EncryptedClientHelloRejectionVerify = func(state tls.ConnectionState) error {
		if !state.ECHAccepted {
			return ErrECHServerReject
		}
		return nil
	}
	return true
}

func resolveIPByType(host string, _type uint16) (netip.Addr, error) {
	var (
		ips []netip.Addr
		err error
	)

	switch _type {
	case typeNone:
		ips, err = LookupIP(context.Background(), host)
	case typeA:
		ips, err = LookupIPv4(context.Background(), host)
	default:
		ips, err = LookupIPv6(context.Background(), host)
	}

	if err != nil {
		return netip.Addr{}, err
	}

	return ips[rand.IntN(len(ips))], nil
}

func resolveProxyServerHostByType(host string, _type uint16) (netip.Addr, error) {
	var (
		ips []netip.Addr
		err error
		ctx = context.WithValue(context.Background(), proxyServerHostKey, struct{}{})
	)

	ips, err = lookupIPByResolverAndType(ctx, host, DefaultResolver, _type, needProxyHostIPv6)
	if err != nil {
		return netip.Addr{}, err
	}

	return ips[rand.IntN(len(ips))], nil
}

func lookupIPByProxyAndType(ctx context.Context, host, proxy string, t uint16, both bool) ([]netip.Addr, error) {
	ctx = context.WithValue(ctx, proxyKey, proxy)
	return lookupIPByResolverAndType(ctx, host, DefaultResolver, t, both)
}

func lookupIPByResolverAndType(ctx context.Context, host string, r Resolver, t uint16, both bool) ([]netip.Addr, error) {
	if t == typeAAAA && DisableIPv6 && !both {
		return nil, ErrIPv6Disabled
	}

	if node := DefaultHosts.Search(host); node != nil {
		ip := node.Data
		if t != typeAAAA {
			ip = ip.Unmap()
		}
		if t == typeNone || (t == typeA && ip.Is4()) || (t == typeAAAA && ip.Is6()) {
			return []netip.Addr{ip}, nil
		}
	}

	if r != nil {
		if t == typeA {
			return r.LookupIPv4(ctx, host)
		} else if t == typeAAAA {
			return r.LookupIPv6(ctx, host)
		}
		if DisableIPv6 && !both {
			return r.LookupIPv4(ctx, host)
		}
		return r.LookupIP(ctx, host)
	} else if t == typeNone && DisableIPv6 {
		return LookupIPv4(ctx, host)
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if t != typeAAAA {
			ip = ip.Unmap()
		}
		is4 := ip.Is4()
		if (t == typeA && !is4) || (t == typeAAAA && is4) {
			return nil, ErrIPVersion
		}
		return []netip.Addr{ip}, nil
	}

	network := "ip"
	if t == typeA {
		network = "ip4"
	} else if t == typeAAAA {
		network = "ip6"
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, network, host)
	if err != nil {
		return nil, err
	} else if len(ips) == 0 {
		return nil, ErrIPNotFound
	}

	return lo.Map(ips, func(item net.IP, _ int) netip.Addr {
		ip, _ := netip.AddrFromSlice(item)
		if t != typeAAAA {
			ip = ip.Unmap()
		}
		return ip
	}), nil
}
