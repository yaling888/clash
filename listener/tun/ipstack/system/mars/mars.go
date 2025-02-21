package mars

import (
	"net/netip"

	dev "github.com/yaling888/quirktiva/listener/tun/device"
	"github.com/yaling888/quirktiva/listener/tun/ipstack/system/mars/nat"
)

type StackListener struct {
	device dev.Device
	tcp    *nat.TCP
	udp    *nat.UDP
}

func StartListener(device dev.Device, gateway, portal, gateway6, portal6 netip.Addr) (*StackListener, error) {
	tcp, udp, err := nat.Start(device, gateway, portal, gateway6, portal6)
	if err != nil {
		return nil, err
	}

	return &StackListener{
		device: device,
		tcp:    tcp,
		udp:    udp,
	}, nil
}

func (t *StackListener) Close() error {
	_ = t.udp.Close()
	return t.tcp.Close()
}

func (t *StackListener) TCP() *nat.TCP {
	return t.tcp
}

func (t *StackListener) UDP() *nat.UDP {
	return t.udp
}
