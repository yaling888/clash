package outbound

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/yaling888/quirktiva/common/convert"
	"github.com/yaling888/quirktiva/component/dialer"
	"github.com/yaling888/quirktiva/component/resolver"
	C "github.com/yaling888/quirktiva/constant"
)

var _ C.ProxyAdapter = (*Http)(nil)

type Http struct {
	*Base
	user      string
	pass      string
	useECH    bool
	tlsConfig *tls.Config
	headers   http.Header
}

type HttpOption struct {
	BasicOption
	Name             string            `proxy:"name"`
	Server           string            `proxy:"server"`
	Port             int               `proxy:"port"`
	UserName         string            `proxy:"username,omitempty"`
	Password         string            `proxy:"password,omitempty"`
	TLS              bool              `proxy:"tls,omitempty"`
	SNI              string            `proxy:"sni,omitempty"`
	SkipCertVerify   bool              `proxy:"skip-cert-verify,omitempty"`
	Headers          map[string]string `proxy:"headers,omitempty"`
	RemoteDnsResolve bool              `proxy:"remote-dns-resolve,omitempty"`
}

// StreamConn implements C.ProxyAdapter
func (h *Http) StreamConn(c net.Conn, metadata *C.Metadata) (net.Conn, error) {
	if h.tlsConfig != nil {
		var cc *tls.Conn
		if h.useECH {
			tlsConfig := copyTLSConfig(h.tlsConfig)
			h.useECH = resolver.SetECHConfigList(tlsConfig)
			cc = tls.Client(c, tlsConfig)
		} else {
			cc = tls.Client(c, h.tlsConfig)
		}
		ctx, cancel := context.WithTimeout(context.Background(), C.DefaultTLSTimeout)
		defer cancel()
		err := cc.HandshakeContext(ctx)
		c = cc
		if err != nil {
			return nil, fmt.Errorf("%s connect error: %w", h.addr, err)
		}
	}

	if err := h.shakeHand(metadata, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DialContext implements C.ProxyAdapter
func (h *Http) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.Conn, err error) {
	c, err := dialer.DialContext(ctx, "tcp", h.addr, h.Base.DialOptions(opts...)...)
	if err != nil {
		return nil, fmt.Errorf("%s connect error: %w", h.addr, err)
	}
	tcpKeepAlive(c)

	defer func(cc net.Conn, e error) {
		safeConnClose(cc, e)
	}(c, err)

	c, err = h.StreamConn(c, metadata)
	if err != nil {
		return nil, err
	}

	return NewConn(c, h), nil
}

func (h *Http) shakeHand(metadata *C.Metadata, rw io.ReadWriter) error {
	addr := metadata.RemoteAddress()
	req := &http.Request{
		Method: http.MethodConnect,
		URL: &url.URL{
			Host: addr,
		},
		Host:   addr,
		Header: h.headers.Clone(),
	}

	req.Header.Add("Proxy-Connection", "Keep-Alive")

	if h.user != "" && h.pass != "" {
		auth := h.user + ":" + h.pass
		req.Header.Add("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
	}

	if metadata.Type == C.MITM {
		req.Header.Set("Origin-Request-Source-Address", metadata.SourceAddress())
		req.Header.Set("Origin-Request-Special-Proxy", metadata.SpecialProxy)
	}

	if err := req.Write(rw); err != nil {
		return err
	}

	resp, err := http.ReadResponse(bufio.NewReader(rw), req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusProxyAuthRequired {
		return errors.New("HTTP need auth")
	}

	if resp.StatusCode == http.StatusMethodNotAllowed {
		return errors.New("CONNECT method not allowed by proxy")
	}

	if resp.StatusCode >= http.StatusInternalServerError {
		return errors.New(resp.Status)
	}

	return fmt.Errorf("can not connect remote err code: %d", resp.StatusCode)
}

func NewHttp(option HttpOption) *Http {
	var tlsConfig *tls.Config
	if option.TLS {
		sni := option.Server
		if option.SNI != "" {
			sni = option.SNI
		}
		tlsConfig = &tls.Config{
			InsecureSkipVerify: option.SkipCertVerify,
			ServerName:         sni,
		}
	}

	headers := http.Header{}
	for name, value := range option.Headers {
		headers.Add(name, value)
	}

	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", convert.RandUserAgent())
	}

	return &Http{
		Base: &Base{
			name:  option.Name,
			addr:  net.JoinHostPort(option.Server, strconv.Itoa(option.Port)),
			tp:    C.Http,
			iface: option.Interface,
			rmark: option.RoutingMark,
			dns:   option.RemoteDnsResolve,
		},
		user:      option.UserName,
		pass:      option.Password,
		tlsConfig: tlsConfig,
		headers:   headers,
		useECH:    true,
	}
}
