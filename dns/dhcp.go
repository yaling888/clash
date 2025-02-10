package dns

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	D "github.com/miekg/dns"

	"github.com/yaling888/quirktiva/component/dhcp"
	"github.com/yaling888/quirktiva/component/iface"
	"github.com/yaling888/quirktiva/component/resolver"
)

const (
	IfaceTTL    = time.Second * 20
	DHCPTTL     = time.Hour
	DHCPTimeout = time.Minute
)

var _ dnsClient = (*dhcpClient)(nil)

type dhcpClient struct {
	ifaceName string

	lock            sync.Mutex
	ifaceInvalidate time.Time
	dnsInvalidate   time.Time

	ifaceAddr *netip.Prefix
	done      chan struct{}
	clients   []dnsClient
	err       error
}

func (d *dhcpClient) IsLan() bool {
	return false
}

func (d *dhcpClient) Exchange(m *D.Msg) (msg *rMsg, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), resolver.DefaultDNSTimeout)
	defer cancel()

	return d.ExchangeContext(ctx, m)
}

func (d *dhcpClient) ExchangeContext(ctx context.Context, m *D.Msg) (msg *rMsg, err error) {
	if m.Question[0].Qtype == D.TypeHTTPS {
		return nil, resolver.ErrECHNotSupport
	}

	clients, err := d.resolve(ctx)
	if err != nil {
		return nil, err
	}

	return batchExchange(ctx, clients, m)
}

func (d *dhcpClient) resolve(ctx context.Context) ([]dnsClient, error) {
	d.lock.Lock()

	invalidated, err := d.invalidate()
	if err != nil {
		d.err = err
	} else if invalidated {
		done := make(chan struct{})

		d.done = done

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), DHCPTimeout)
			defer cancel()

			var res []dnsClient
			dns, err := dhcp.ResolveDNSFromDHCP(ctx, d.ifaceName)
			// dns never empty if err is nil
			if err == nil {
				nameserver := make([]NameServer, 0, len(dns))
				for _, item := range dns {
					nameserver = append(nameserver, NameServer{
						Addr:      net.JoinHostPort(item.String(), "53"),
						Interface: d.ifaceName,
						IsDHCP:    true,
					})
				}

				res = transform(nameserver, nil)
			}

			d.lock.Lock()
			defer d.lock.Unlock()

			close(done)

			d.done = nil
			d.clients = res
			d.err = err
		}()
	}

	d.lock.Unlock()

	for {
		d.lock.Lock()

		res, err, done := d.clients, d.err, d.done

		d.lock.Unlock()

		// initializing
		if res == nil && err == nil {
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// dirty return
		return res, err
	}
}

func (d *dhcpClient) invalidate() (bool, error) {
	if time.Now().Before(d.ifaceInvalidate) {
		return false, nil
	}

	d.ifaceInvalidate = time.Now().Add(IfaceTTL)

	ifaceObj, err := iface.ResolveInterface(d.ifaceName)
	if err != nil {
		return false, err
	}

	addr, err := ifaceObj.PickIPv4Addr(netip.Addr{})
	if err != nil {
		return false, err
	}

	if time.Now().Before(d.dnsInvalidate) && d.ifaceAddr == addr {
		return false, nil
	}

	d.dnsInvalidate = time.Now().Add(DHCPTTL)
	d.ifaceAddr = addr

	return d.done == nil, nil
}

func newDHCPClient(ifaceName string) *dhcpClient {
	return &dhcpClient{ifaceName: ifaceName}
}
