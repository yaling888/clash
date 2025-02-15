package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/phuslu/log"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/yaling888/quirktiva/adapter"
	"github.com/yaling888/quirktiva/adapter/outbound"
	"github.com/yaling888/quirktiva/adapter/outboundgroup"
	"github.com/yaling888/quirktiva/adapter/provider"
	"github.com/yaling888/quirktiva/component/auth"
	"github.com/yaling888/quirktiva/component/fakeip"
	"github.com/yaling888/quirktiva/component/geodata"
	"github.com/yaling888/quirktiva/component/geodata/router"
	_ "github.com/yaling888/quirktiva/component/geodata/standard"
	"github.com/yaling888/quirktiva/component/resolver"
	S "github.com/yaling888/quirktiva/component/script"
	"github.com/yaling888/quirktiva/component/trie"
	C "github.com/yaling888/quirktiva/constant"
	providerTypes "github.com/yaling888/quirktiva/constant/provider"
	"github.com/yaling888/quirktiva/dns"
	L "github.com/yaling888/quirktiva/log"
	"github.com/yaling888/quirktiva/mitm"
	R "github.com/yaling888/quirktiva/rule"
	T "github.com/yaling888/quirktiva/tunnel"
)

// General config
type General struct {
	LegacyInbound
	Controller
	Authentication []string     `json:"authentication"`
	Mode           T.TunnelMode `json:"mode"`
	LogLevel       L.LogLevel   `json:"log-level"`
	IPv6           bool         `json:"ipv6"`
	Sniffing       bool         `json:"sniffing"`
	Interface      string       `json:"-"`
	RoutingMark    int          `json:"-"`
	Tun            C.Tun        `json:"tun"`
	EBpf           EBpf         `json:"-"`
}

// LegacyInbound config
type LegacyInbound struct {
	Port        int    `json:"port"`
	SocksPort   int    `json:"socks-port"`
	RedirPort   int    `json:"redir-port"`
	TProxyPort  int    `json:"tproxy-port"`
	MixedPort   int    `json:"mixed-port"`
	MitmPort    int    `json:"mitm-port"`
	AllowLan    bool   `json:"allow-lan"`
	BindAddress string `json:"bind-address"`
}

// Controller config
type Controller struct {
	ExternalController string `json:"-"`
	ExternalUI         string `json:"-"`
	ExternalServerName string `json:"-"`
	Secret             string `json:"-"`
	PPROF              bool   `json:"-"`
}

// DNS config
type DNS struct {
	Enable                bool             `yaml:"enable"`
	IPv6                  bool             `yaml:"ipv6"`
	RemoteDnsResolve      bool             `yaml:"remote-dns-resolve"`
	NameServer            []dns.NameServer `yaml:"nameserver"`
	Fallback              []dns.NameServer `yaml:"fallback"`
	ProxyServerNameserver []dns.NameServer `yaml:"proxy-server-nameserver"`
	RemoteNameserver      []dns.NameServer `yaml:"remote-nameserver"`
	FallbackFilter        FallbackFilter   `yaml:"fallback-filter"`
	Listen                string           `yaml:"listen"`
	EnhancedMode          C.DNSMode        `yaml:"enhanced-mode"`
	DefaultNameserver     []dns.NameServer `yaml:"default-nameserver"`
	FakeIPRange           *fakeip.Pool
	Hosts                 *trie.DomainTrie[netip.Addr]
	NameServerPolicy      map[string]dns.NameServer
	SearchDomains         []string
}

// FallbackFilter config
type FallbackFilter struct {
	GeoIP     bool                    `yaml:"geoip"`
	GeoIPCode string                  `yaml:"geoip-code"`
	IPCIDR    []*netip.Prefix         `yaml:"ipcidr"`
	Domain    []string                `yaml:"domain"`
	GeoSite   []*router.DomainMatcher `yaml:"geosite"`
}

// Profile config
type Profile struct {
	StoreSelected bool `yaml:"store-selected"`
	StoreFakeIP   bool `yaml:"store-fake-ip"`
	Tracing       bool `yaml:"tracing"`
}

// Script config
type Script struct {
	Engine        string            `yaml:"engine" json:"engine"`
	MainCode      string            `yaml:"code" json:"code"`
	MainPath      string            `yaml:"path" json:"path"`
	ShortcutsCode map[string]string `yaml:"shortcuts" json:"shortcuts"`
}

// Mitm config
type Mitm struct {
	Hosts *trie.DomainTrie[bool] `yaml:"hosts" json:"hosts"`
	Rules C.RewriteRule          `yaml:"rules" json:"rules"`
}

// EBpf config
type EBpf struct {
	RedirectToTun []string `yaml:"redirect-to-tun" json:"redirect-to-tun"`
	AutoRedir     []string `yaml:"auto-redir" json:"auto-redir"`
}

// Experimental config
type Experimental struct {
	UDPFallbackMatch  bool   `yaml:"udp-fallback-match"`
	UDPFallbackPolicy string `yaml:"udp-fallback-policy"`
}

// Config is clash config manager
type Config struct {
	General       *General
	Mitm          *Mitm
	DNS           *DNS
	Experimental  *Experimental
	Hosts         *trie.DomainTrie[netip.Addr]
	Profile       *Profile
	Inbounds      []C.Inbound
	Rules         []C.Rule
	RuleProviders map[string]C.Rule
	Users         []auth.AuthUser
	Proxies       map[string]C.Proxy
	Providers     map[string]providerTypes.ProxyProvider
	MainMatcher   C.Matcher
	Tunnels       []Tunnel
}

type RawDNS struct {
	Enable                bool              `yaml:"enable"`
	IPv6                  *bool             `yaml:"ipv6"`
	RemoteDnsResolve      bool              `yaml:"remote-dns-resolve"`
	UseHosts              bool              `yaml:"use-hosts"`
	NameServer            []string          `yaml:"nameserver"`
	Fallback              []string          `yaml:"fallback"`
	FallbackFilter        RawFallbackFilter `yaml:"fallback-filter"`
	Listen                string            `yaml:"listen"`
	EnhancedMode          C.DNSMode         `yaml:"enhanced-mode"`
	FakeIPRange           string            `yaml:"fake-ip-range"`
	FakeIPFilter          []string          `yaml:"fake-ip-filter"`
	DefaultNameserver     []string          `yaml:"default-nameserver"`
	NameServerPolicy      map[string]string `yaml:"nameserver-policy"`
	ProxyServerNameserver []string          `yaml:"proxy-server-nameserver"`
	SearchDomains         []string          `yaml:"search-domains"`
	RemoteNameserver      []string          `yaml:"remote-nameserver"`
}

type RawFallbackFilter struct {
	GeoIP     bool     `yaml:"geoip"`
	GeoIPCode string   `yaml:"geoip-code"`
	IPCIDR    []string `yaml:"ipcidr"`
	Domain    []string `yaml:"domain"`
	GeoSite   []string `yaml:"geosite"`
}

type RawMitm struct {
	Hosts []string `yaml:"hosts" json:"hosts"`
	Rules []string `yaml:"rules" json:"rules"`
}

type tunnel struct {
	Network []string `yaml:"network"`
	Address string   `yaml:"address"`
	Target  string   `yaml:"target"`
	Proxy   string   `yaml:"proxy"`
}

type Tunnel tunnel

// UnmarshalYAML implements yaml.Unmarshaler
func (t *Tunnel) UnmarshalYAML(unmarshal func(any) error) error {
	var tp string
	if err := unmarshal(&tp); err != nil {
		var inner tunnel
		if err := unmarshal(&inner); err != nil {
			return err
		}

		*t = Tunnel(inner)
		return nil
	}

	// parse udp/tcp,address,target,proxy
	parts := lo.Map(strings.Split(tp, ","), func(s string, _ int) string {
		return strings.TrimSpace(s)
	})
	if len(parts) != 4 {
		return fmt.Errorf("invalid tunnel config %s", tp)
	}
	network := strings.Split(parts[0], "/")

	// validate network
	for _, n := range network {
		switch n {
		case "tcp", "udp":
		default:
			return fmt.Errorf("invalid tunnel network %s", n)
		}
	}

	// validate address and target
	address := parts[1]
	target := parts[2]
	for _, addr := range []string{address, target} {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("invalid tunnel target or address %s", addr)
		}
	}

	*t = Tunnel(tunnel{
		Network: network,
		Address: address,
		Target:  target,
		Proxy:   parts[3],
	})
	return nil
}

type rawRule struct {
	If     string    `yaml:"if"`
	Name   string    `yaml:"name"`
	Engine string    `yaml:"engine"`
	Rules  []RawRule `yaml:"rules"`
	Line   string    `yaml:"-"`
}

type RawRule rawRule

// UnmarshalYAML implements yaml.Unmarshaler
func (r *RawRule) UnmarshalYAML(unmarshal func(any) error) error {
	var line string
	if err := unmarshal(&line); err != nil {
		inner := rawRule{
			Engine: "expr",
		}
		if err = unmarshal(&inner); err != nil {
			return err
		}

		if inner.Name == "" {
			return fmt.Errorf("invalid rule name")
		}

		if inner.If == "" {
			return fmt.Errorf("invalid rule %s if", inner.Name)
		}

		if inner.Engine != "expr" && inner.Engine != "starlark" {
			return fmt.Errorf("invalid rule %s engine, got %s, want expr or starlark", inner.Name, inner.Engine)
		}

		if len(inner.Rules) == 0 {
			return fmt.Errorf("rule %s sub-rules can not be empty", inner.Name)
		}

		*r = RawRule(inner)
		return nil
	}

	*r = RawRule(rawRule{
		Line: line,
	})
	return nil
}

type RawConfig struct {
	Port               int          `yaml:"port"`
	SocksPort          int          `yaml:"socks-port"`
	RedirPort          int          `yaml:"redir-port"`
	TProxyPort         int          `yaml:"tproxy-port"`
	MixedPort          int          `yaml:"mixed-port"`
	MitmPort           int          `yaml:"mitm-port"`
	Authentication     []string     `yaml:"authentication"`
	AllowLan           bool         `yaml:"allow-lan"`
	BindAddress        string       `yaml:"bind-address"`
	Mode               T.TunnelMode `yaml:"mode"`
	LogLevel           L.LogLevel   `yaml:"log-level"`
	IPv6               bool         `yaml:"ipv6"`
	ExternalController string       `yaml:"external-controller"`
	ExternalUI         string       `yaml:"external-ui"`
	ExternalServerName string       `yaml:"external-server-name"`
	Secret             string       `yaml:"secret"`
	PPROF              bool         `yaml:"pprof"`
	Interface          string       `yaml:"interface-name"`
	RoutingMark        int          `yaml:"routing-mark"`
	Sniffing           bool         `yaml:"sniffing"`
	ForceCertVerify    bool         `yaml:"force-cert-verify"`
	Tunnels            []Tunnel     `yaml:"tunnels"`

	ProxyProvider map[string]map[string]any `yaml:"proxy-providers"`
	Hosts         map[string]string         `yaml:"hosts"`
	Inbounds      []C.Inbound               `yaml:"inbounds"`
	DNS           RawDNS                    `yaml:"dns"`
	Tun           C.Tun                     `yaml:"tun"`
	MITM          RawMitm                   `yaml:"mitm"`
	Experimental  Experimental              `yaml:"experimental"`
	Profile       Profile                   `yaml:"profile"`
	Proxy         []C.RawProxy              `yaml:"proxies"`
	ProxyGroup    []map[string]any          `yaml:"proxy-groups"`
	Rule          []RawRule                 `yaml:"rules"`
	Script        Script                    `yaml:"script"`
	EBpf          EBpf                      `yaml:"ebpf"`
}

// Parse config
func Parse(buf []byte) (*Config, error) {
	rawCfg, err := UnmarshalRawConfig(buf)
	if err != nil {
		return nil, err
	}

	return ParseRawConfig(rawCfg)
}

func UnmarshalRawConfig(buf []byte) (*RawConfig, error) {
	// config with default value
	rawCfg := &RawConfig{
		AllowLan:        false,
		Sniffing:        false,
		ForceCertVerify: false,
		BindAddress:     "*",
		Mode:            T.Rule,
		Authentication:  []string{},
		LogLevel:        L.INFO,
		Hosts:           map[string]string{},
		Rule:            []RawRule{},
		Proxy:           []C.RawProxy{},
		ProxyGroup:      []map[string]any{},
		Tun: C.Tun{
			Enable: false,
			Device: "",
			Stack:  C.TunGvisor,
			DNSHijack: []C.DNSUrl{ // default hijack all dns lookup
				{
					Network: "udp",
					AddrPort: C.DNSAddrPort{
						AddrPort: netip.MustParseAddrPort("0.0.0.0:53"),
					},
				},
				{
					Network: "tcp",
					AddrPort: C.DNSAddrPort{
						AddrPort: netip.MustParseAddrPort("0.0.0.0:53"),
					},
				},
			},
			AutoRoute:           false,
			AutoDetectInterface: false,
		},
		EBpf: EBpf{
			RedirectToTun: []string{},
			AutoRedir:     []string{},
		},
		DNS: RawDNS{
			Enable:           false,
			UseHosts:         true,
			RemoteDnsResolve: true,
			FakeIPRange:      "198.18.0.1/16",
			FallbackFilter: RawFallbackFilter{
				GeoIP:     true,
				GeoIPCode: "CN",
				IPCIDR:    []string{},
				GeoSite:   []string{},
			},
			DefaultNameserver: []string{
				"114.114.114.114",
				"8.8.8.8",
			},
			RemoteNameserver: []string{
				"tcp://1.1.1.1",
				"tcp://8.8.8.8",
			},
		},
		MITM: RawMitm{
			Hosts: []string{},
			Rules: []string{},
		},
		Profile: Profile{
			StoreSelected: true,
			Tracing:       true,
		},
		Script: Script{
			Engine: "expr",
		},
	}

	if err := yaml.Unmarshal(buf, rawCfg); err != nil {
		return nil, err
	}

	return rawCfg, nil
}

func ParseRawConfig(rawCfg *RawConfig) (config *Config, err error) {
	defer func() {
		if err != nil {
			providerTypes.Cleanup(config.Proxies, config.Providers)
			config = nil
		}
		geodata.CleanGeoSiteCache()
		runtime.GC()
	}()

	config = &Config{}

	config.Experimental = &rawCfg.Experimental
	config.Profile = &rawCfg.Profile

	general, err := parseGeneral(rawCfg)
	if err != nil {
		return
	}
	config.General = general

	config.Inbounds = rawCfg.Inbounds

	proxies, providers, err := parseProxies(rawCfg)
	if err != nil {
		return
	}
	config.Proxies = proxies
	config.Providers = providers

	matchers, rawRules, err := parseScript(rawCfg.Script, rawCfg.Rule)
	if err != nil {
		return
	}
	rawCfg.Rule = rawRules
	config.MainMatcher = matchers["main"]

	rules, ruleProviders, err := parseRules(rawCfg, proxies, matchers)
	if err != nil {
		return
	}
	config.Rules = rules
	config.RuleProviders = ruleProviders

	hosts, err := parseHosts(rawCfg)
	if err != nil {
		return
	}
	config.Hosts = hosts

	dnsCfg, err := parseDNS(rawCfg, hosts)
	if err != nil {
		return
	}
	config.DNS = dnsCfg

	mitmCfg, err := parseMitm(rawCfg.MITM)
	if err != nil {
		return
	}
	config.Mitm = mitmCfg

	config.Users = ParseAuthentication(rawCfg.Authentication)

	config.Tunnels = rawCfg.Tunnels
	// verify tunnels
	for _, t := range config.Tunnels {
		if _, ok := config.Proxies[t.Proxy]; !ok {
			pds := config.Providers
		loop:
			for _, pd := range pds {
				if pd.VehicleType() == providerTypes.Compatible {
					continue
				}
				for _, p := range pd.Proxies() {
					ok = p.Name() == t.Proxy
					if ok {
						break loop
					}
				}
			}
			if !ok {
				err = fmt.Errorf("tunnel proxy %s not found", t.Proxy)
				return
			}
		}
	}

	err = verifyScriptMatcher(config, matchers)
	return
}

func parseGeneral(cfg *RawConfig) (*General, error) {
	externalUI := cfg.ExternalUI

	// checkout externalUI exist
	if externalUI != "" {
		externalUI = C.Path.Resolve(externalUI)

		if _, err := os.Stat(externalUI); os.IsNotExist(err) {
			return nil, fmt.Errorf("external-ui: %s not exist", externalUI)
		}
	}

	cfg.Tun.RedirectToTun = cfg.EBpf.RedirectToTun

	return &General{
		LegacyInbound: LegacyInbound{
			Port:        cfg.Port,
			SocksPort:   cfg.SocksPort,
			RedirPort:   cfg.RedirPort,
			TProxyPort:  cfg.TProxyPort,
			MixedPort:   cfg.MixedPort,
			MitmPort:    cfg.MitmPort,
			AllowLan:    cfg.AllowLan,
			BindAddress: cfg.BindAddress,
		},
		Controller: Controller{
			ExternalController: cfg.ExternalController,
			ExternalUI:         cfg.ExternalUI,
			ExternalServerName: cfg.ExternalServerName,
			Secret:             cfg.Secret,
			PPROF:              cfg.PPROF,
		},
		Mode:        cfg.Mode,
		LogLevel:    cfg.LogLevel,
		IPv6:        cfg.IPv6,
		Interface:   cfg.Interface,
		RoutingMark: cfg.RoutingMark,
		Sniffing:    cfg.Sniffing,
		Tun:         cfg.Tun,
		EBpf:        cfg.EBpf,
	}, nil
}

func parseProxies(cfg *RawConfig) (proxiesMap map[string]C.Proxy, pdsMap map[string]providerTypes.ProxyProvider, err error) {
	proxies := make(map[string]C.Proxy)
	providersMap := make(map[string]providerTypes.ProxyProvider)
	proxiesConfig := cfg.Proxy
	groupsConfig := cfg.ProxyGroup
	providersConfig := cfg.ProxyProvider
	forceCertVerify := cfg.ForceCertVerify

	var proxyList []string

	proxies["DIRECT"] = adapter.NewProxy(outbound.NewDirect())
	proxies["REJECT"] = adapter.NewProxy(outbound.NewReject())
	proxyList = append(proxyList, "DIRECT", "REJECT")

	defer func() {
		if err != nil {
			providerTypes.Cleanup(proxies, providersMap)
		}
	}()

	// parse proxy
	for idx, mapping := range proxiesConfig {
		mapping.Init()
		proxy, err := adapter.ParseProxy(mapping.M, adapter.ProxyOption{ForceCertVerify: forceCertVerify})
		if err != nil {
			return nil, nil, fmt.Errorf("proxy %d: %w", idx, err)
		}

		if _, exist := proxies[proxy.Name()]; exist {
			return nil, nil, fmt.Errorf("proxy %s is the duplicate name", proxy.Name())
		}
		proxies[proxy.Name()] = proxy
		proxyList = append(proxyList, proxy.Name())
	}

	// keep the original order of ProxyGroups in config file
	for idx, mapping := range groupsConfig {
		groupName, existName := mapping["name"].(string)
		if !existName {
			return nil, nil, fmt.Errorf("proxy group %d: missing name", idx)
		}
		proxyList = append(proxyList, groupName)
	}

	// check if any loop exists and sort the ProxyGroups
	if err := proxyGroupsDagSort(groupsConfig); err != nil {
		return nil, nil, err
	}

	// parse and initial providers
	for name, mapping := range providersConfig {
		if name == provider.ReservedName {
			return nil, nil, fmt.Errorf(
				"can not defined a provider called `%s`", provider.ReservedName,
			)
		}

		pd, err := provider.ParseProxyProvider(name, mapping, forceCertVerify)
		if err != nil {
			return nil, nil, fmt.Errorf("parse proxy provider %s error: %w", name, err)
		}

		providersMap[name] = pd
	}

	for _, proxyProvider := range providersMap {
		log.Info().Str("name", proxyProvider.Name()).Msg("[Config] initial proxy provider")
		if err := proxyProvider.Initial(); err != nil {
			return nil, nil, fmt.Errorf(
				"initial proxy provider %s error: %w", proxyProvider.Name(), err,
			)
		}
	}

	// parse proxy group
	for idx, mapping := range groupsConfig {
		group, err := outboundgroup.ParseProxyGroup(mapping, proxies, providersMap)
		if err != nil {
			return nil, nil, fmt.Errorf("proxy group[%d]: %w", idx, err)
		}

		groupName := group.Name()
		if _, exist := proxies[groupName]; exist {
			return nil, nil, fmt.Errorf("proxy group %s: the duplicate name", groupName)
		}

		proxies[groupName] = adapter.NewProxy(group)
	}

	// initial compatible provider
	for _, pd := range providersMap {
		if pd.VehicleType() != providerTypes.Compatible {
			continue
		}

		log.Info().Str("name", pd.Name()).Msg("[Config] initial compatible provider")
		if err := pd.Initial(); err != nil {
			return nil, nil, fmt.Errorf(
				"initial compatible provider %s error: %w", pd.Name(), err,
			)
		}
	}

	var ps []C.Proxy
	for _, v := range proxyList {
		ps = append(ps, proxies[v])
	}
	hc := provider.NewHealthCheck(ps, "", 0, true)
	pd, _ := provider.NewCompatibleProvider(provider.ReservedName, hc, nil)
	pd.SetProxies(ps)
	providersMap[provider.ReservedName] = pd

	global := outboundgroup.NewSelector(
		&outboundgroup.GroupCommonOption{
			Name: "GLOBAL",
		},
		[]providerTypes.ProxyProvider{pd},
	)
	proxies["GLOBAL"] = adapter.NewProxy(global)
	return proxies, providersMap, nil
}

func parseRules(cfg *RawConfig, proxies map[string]C.Proxy, matchers map[string]C.Matcher) ([]C.Rule, map[string]C.Rule, error) {
	defer runtime.GC()

	ruleProviders := make(map[string]C.Rule)

	rules, err := parseRawRules(cfg.Rule, ruleProviders, proxies, matchers)
	if err != nil {
		return nil, nil, err
	}

	return rules, ruleProviders, nil
}

func parseRawRules(
	rawRules []RawRule,
	ruleProviders map[string]C.Rule,
	proxies map[string]C.Proxy,
	matchers map[string]C.Matcher,
) ([]C.Rule, error) {
	rules := make([]C.Rule, 0, len(rawRules))

	for idx, raw := range rawRules {
		line := raw.Line
		if line == "" {
			if raw.Name == "" {
				continue
			}

			mk := "rule:" + raw.Name
			if _, ok := matchers[mk]; ok {
				return nil, fmt.Errorf("parse rule %s failed, rule name is exist", raw.Name)
			}

			var (
				groupMatcher C.Matcher
				err          error
			)

			if raw.Engine == "expr" {
				groupMatcher, err = S.NewExprMatcher(raw.Name, raw.If)
			} else {
				groupMatcher, err = S.NewMatcher(raw.Name, "", raw.If)
			}

			if err != nil {
				return nil, fmt.Errorf("parse rule %s failed, %w", raw.Name, err)
			}

			matchers[mk] = groupMatcher

			subRules, err := parseRawRules(raw.Rules, ruleProviders, proxies, matchers)
			if err != nil {
				return nil, err
			}

			appendRuleGroupName(subRules, raw.Name)

			parsed := R.NewGroup(fmt.Sprintf("%s (%s)", raw.Name, raw.If), groupMatcher, subRules)

			rules = append(rules, parsed)

			rpdArr := findRuleProvidersName(raw.If)
			for _, v := range rpdArr {
				v = strings.ToLower(v)
				if _, ok := ruleProviders[v]; ok {
					continue
				}
				rpd, err := R.NewGEOSITE(v, C.ScriptRuleGeoSiteTarget)
				if err != nil {
					continue
				}
				ruleProviders[v] = rpd
			}
			continue
		}

		rule := trimArr(strings.Split(line, ","))
		var (
			payload  string
			target   string
			params   []string
			ruleName = strings.ToUpper(rule[0])
		)

		l := len(rule)

		if l < 2 {
			return nil, fmt.Errorf("rules[%d] [%s] error: format invalid", idx, line)
		}

		if l < 4 {
			rule = append(rule, make([]string, 4-l)...)
		}

		if ruleName == "MATCH" {
			l = 2
		}

		if l >= 3 {
			l = 3
			payload = rule[1]
		}

		target = rule[l-1]
		params = rule[l:]

		if _, ok := proxies[target]; !ok && ruleName != "GEOSITE" && target != C.ScriptRuleGeoSiteTarget {
			return nil, fmt.Errorf("rules[%d] [%s] error: proxy [%s] not found", idx, line, target)
		}

		pvName := strings.ToLower(payload)
		_, foundRP := ruleProviders[pvName]
		if ruleName == "GEOSITE" && target == C.ScriptRuleGeoSiteTarget && foundRP {
			continue
		}

		params = trimArr(params)

		parsed, parseErr := R.ParseRule(ruleName, payload, target, params)
		if parseErr != nil {
			return nil, fmt.Errorf("rules[%d] [%s] error: %w", idx, line, parseErr)
		}

		if scr, ok := parsed.(*R.Script); ok {
			m := matchers[payload]
			if m == nil {
				return nil, fmt.Errorf(
					"rules[%d] [%s] error: shortcut name [%s] not found", idx, line, payload,
				)
			}
			scr.SetMatcher(m)
		}

		if ruleName == "GEOSITE" && !foundRP {
			ruleProviders[pvName] = parsed
		}

		rules = append(rules, parsed)
	}

	return rules, nil
}

func appendRuleGroupName(subRules []C.Rule, groupName string) {
	for i := range subRules {
		subRules[i].AppendGroup(groupName)
		if subRules[i].RuleType() != C.Group {
			continue
		}
		appendRuleGroupName(subRules[i].SubRules(), groupName)
	}
}

func parseHosts(cfg *RawConfig) (*trie.DomainTrie[netip.Addr], error) {
	tree := trie.New[netip.Addr]()

	// add default hosts
	if err := tree.Insert("localhost", netip.AddrFrom4([4]byte{127, 0, 0, 1})); err != nil {
		log.Error().Err(err).Msg("[Config] insert localhost to host failed")
	}

	if len(cfg.Hosts) != 0 {
		for domain, ipStr := range cfg.Hosts {
			ip, err := netip.ParseAddr(ipStr)
			if err != nil {
				return nil, fmt.Errorf("%s is not a valid IP", ipStr)
			}
			_ = tree.Insert(domain, ip)
		}
	}

	// add mitm API host
	if err := tree.Insert(C.MitmApiHost, netip.AddrFrom4([4]byte{1, 2, 3, 4})); err != nil {
		log.Error().Err(err).Msg("[Config] insert mitm API host to host failed")
	}

	return tree, nil
}

func hostWithDefaultPort(host string, defPort string) (string, error) {
	if !strings.Contains(host, ":") {
		host += ":"
	}

	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		return "", err
	}

	if port == "" {
		port = defPort
	}

	return net.JoinHostPort(hostname, port), nil
}

func parseNameServer(servers []string) ([]dns.NameServer, error) {
	var nameservers []dns.NameServer

	for idx, server := range servers {
		// parse without scheme .e.g 8.8.8.8:53
		if !strings.Contains(server, "://") {
			server = "udp://" + server
		}
		u, err := url.Parse(server)
		if err != nil {
			return nil, fmt.Errorf("DNS NameServer[%d] format error: %w", idx, err)
		}

		var addr, dnsNetType string
		switch u.Scheme {
		case "udp":
			addr, err = hostWithDefaultPort(u.Host, "53")
			dnsNetType = "" // UDP
		case "tcp":
			addr, err = hostWithDefaultPort(u.Host, "53")
			dnsNetType = "tcp" // TCP
		case "tls":
			addr, err = hostWithDefaultPort(u.Host, "853")
			dnsNetType = "tcp-tls" // DNS over TLS
		case "https", "doh":
			clearURL := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, User: u.User}
			addr = clearURL.String()
			dnsNetType = "https" // DNS over HTTPS
		case "http3", "h3", "doh3":
			clearURL := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, User: u.User}
			addr = clearURL.String()
			dnsNetType = "http3" // force DNS over HTTP3
		case "dhcp":
			addr = u.Host
			dnsNetType = "dhcp" // UDP from DHCP
		default:
			return nil, fmt.Errorf("DNS NameServer[%d] unsupport scheme: %s", idx, u.Scheme)
		}

		if err != nil {
			return nil, fmt.Errorf("DNS NameServer[%d] format error: %w", idx, err)
		}

		nameservers = append(
			nameservers,
			dns.NameServer{
				Net:   dnsNetType,
				Addr:  addr,
				Proxy: u.Fragment,
			},
		)
	}
	return nameservers, nil
}

func parseNameServerPolicy(nsPolicy map[string]string) (map[string]dns.NameServer, error) {
	policy := map[string]dns.NameServer{}

	for domain, server := range nsPolicy {
		nameservers, err := parseNameServer([]string{server})
		if err != nil {
			return nil, err
		}
		if _, valid := trie.ValidAndSplitDomain(domain); !valid {
			return nil, fmt.Errorf("DNS ResoverRule invalid domain: %s", domain)
		}
		policy[domain] = nameservers[0]
	}

	return policy, nil
}

func parseFallbackIPCIDR(ips []string) ([]*netip.Prefix, error) {
	var ipNets []*netip.Prefix

	for idx, ip := range ips {
		ipnet, err := netip.ParsePrefix(ip)
		if err != nil {
			return nil, fmt.Errorf("DNS FallbackIP[%d] format error: %w", idx, err)
		}
		ipNets = append(ipNets, &ipnet)
	}

	return ipNets, nil
}

func parseFallbackGeoSite(countries []string) ([]*router.DomainMatcher, error) {
	var sites []*router.DomainMatcher
	for _, country := range countries {
		matcher, recordsCount, err := geodata.LoadProviderByCode(country)
		if err != nil {
			return nil, err
		}

		sites = append(sites, matcher)

		cont := fmt.Sprintf("%d", recordsCount)
		if recordsCount == 0 {
			cont = "from cache"
		}
		log.Info().
			Str("country", country).
			Str("records", cont).
			Msg("[Config] initial GeoSite dns fallback filter")
	}
	return sites, nil
}

func parseDNS(rawCfg *RawConfig, hosts *trie.DomainTrie[netip.Addr]) (*DNS, error) {
	cfg := rawCfg.DNS
	if cfg.Enable && len(cfg.NameServer) == 0 {
		return nil, fmt.Errorf("if DNS configuration is turned on, NameServer cannot be empty")
	}

	dnsCfg := &DNS{
		Enable:           cfg.Enable,
		Listen:           cfg.Listen,
		IPv6:             lo.FromPtrOr(cfg.IPv6, rawCfg.IPv6),
		EnhancedMode:     cfg.EnhancedMode,
		RemoteDnsResolve: cfg.RemoteDnsResolve,
		FallbackFilter: FallbackFilter{
			IPCIDR:  []*netip.Prefix{},
			GeoSite: []*router.DomainMatcher{},
		},
	}
	var err error
	if dnsCfg.NameServer, err = parseNameServer(cfg.NameServer); err != nil {
		return nil, err
	}

	if dnsCfg.Fallback, err = parseNameServer(cfg.Fallback); err != nil {
		return nil, err
	}

	if dnsCfg.NameServerPolicy, err = parseNameServerPolicy(cfg.NameServerPolicy); err != nil {
		return nil, err
	}

	if dnsCfg.ProxyServerNameserver, err = parseNameServer(cfg.ProxyServerNameserver); err != nil {
		return nil, err
	}

	if cfg.RemoteDnsResolve && len(cfg.RemoteNameserver) == 0 {
		return nil, errors.New(
			"remote nameserver should have at least one nameserver when `remote-dns-resolve` is enable",
		)
	}
	if dnsCfg.RemoteNameserver, err = parseNameServer(cfg.RemoteNameserver); err != nil {
		return nil, err
	}
	// check remote nameserver should not include any dhcp client
	for _, ns := range dnsCfg.RemoteNameserver {
		if ns.Net == "dhcp" {
			return nil, errors.New("remote nameserver should not contain any dhcp client")
		}
	}

	if len(cfg.DefaultNameserver) == 0 {
		return nil, errors.New("default nameserver should have at least one nameserver")
	}
	if dnsCfg.DefaultNameserver, err = parseNameServer(cfg.DefaultNameserver); err != nil {
		return nil, err
	}
	// check default nameserver is pure ip addr
	for _, ns := range dnsCfg.DefaultNameserver {
		host, _, err := net.SplitHostPort(ns.Addr)
		if err != nil || net.ParseIP(host) == nil {
			return nil, errors.New("default nameserver should be pure IP")
		}
	}

	if cfg.EnhancedMode == C.DNSFakeIP {
		ipnet, err := netip.ParsePrefix(cfg.FakeIPRange)
		if err != nil {
			return nil, err
		}

		defaultFakeIPFilter := []string{
			"*.lan",
			"*.local",
			"*.localhost",
			"*.test",
			"+.msftconnecttest.com",
			"localhost.ptlogin2.qq.com",
			"localhost.sec.qq.com",
		}

		// add all nameserver host to fake ip skip host filter
		defaultFakeIPFilter = append(defaultFakeIPFilter,
			lo.Filter(
				lo.Map(
					append(dnsCfg.NameServer,
						append(dnsCfg.Fallback,
							append(dnsCfg.ProxyServerNameserver, dnsCfg.RemoteNameserver...)...)...),
					func(ns dns.NameServer, _ int) string {
						h, _, _ := net.SplitHostPort(ns.Addr)
						if _, err := netip.ParseAddr(h); err != nil {
							return h
						}
						return ""
					}),
				func(s string, _ int) bool {
					return s != ""
				},
			)...,
		)

		// add policy to fake ip skip host filter
		if len(dnsCfg.NameServerPolicy) != 0 {
			for key, policy := range dnsCfg.NameServerPolicy {
				h, _, _ := net.SplitHostPort(policy.Addr)

				if a, err := netip.ParseAddr(h); err != nil {
					defaultFakeIPFilter = append(defaultFakeIPFilter, h)
				} else if a.IsLoopback() || a.IsPrivate() {
					defaultFakeIPFilter = append(defaultFakeIPFilter, key)
				}
			}
		}

		host := trie.New[bool]()

		// fake ip skip host filter
		fakeIPFilter := lo.Uniq(append(cfg.FakeIPFilter, defaultFakeIPFilter...))
		for _, domain := range fakeIPFilter {
			_ = host.Insert(domain, true)
		}

		resolver.StoreFakePoolState()

		pool, err := fakeip.New(fakeip.Options{
			IPNet:       &ipnet,
			Size:        1000,
			Host:        host,
			Persistence: rawCfg.Profile.StoreFakeIP,
		})
		if err != nil {
			return nil, err
		}

		dnsCfg.FakeIPRange = pool
	}

	if len(cfg.Fallback) != 0 {
		dnsCfg.FallbackFilter.GeoIP = cfg.FallbackFilter.GeoIP
		dnsCfg.FallbackFilter.GeoIPCode = cfg.FallbackFilter.GeoIPCode
		if fallbackip, err := parseFallbackIPCIDR(cfg.FallbackFilter.IPCIDR); err == nil {
			dnsCfg.FallbackFilter.IPCIDR = fallbackip
		}
		dnsCfg.FallbackFilter.Domain = cfg.FallbackFilter.Domain
		fallbackGeoSite, err := parseFallbackGeoSite(cfg.FallbackFilter.GeoSite)
		if err != nil {
			return nil, fmt.Errorf("load GeoSite dns fallback filter error, %w", err)
		}
		dnsCfg.FallbackFilter.GeoSite = fallbackGeoSite
	}

	if cfg.UseHosts {
		dnsCfg.Hosts = hosts
	}

	if len(cfg.SearchDomains) != 0 {
		for _, domain := range cfg.SearchDomains {
			if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
				return nil, errors.New("search domains should not start or end with '.'")
			}
			if strings.Contains(domain, ":") {
				return nil, errors.New("search domains are for ipv4 only and should not contain ports")
			}
		}
		dnsCfg.SearchDomains = cfg.SearchDomains
	}

	return dnsCfg, nil
}

func ParseAuthentication(rawRecords []string) []auth.AuthUser {
	var users []auth.AuthUser
	for _, line := range rawRecords {
		if user, pass, found := strings.Cut(line, ":"); found {
			users = append(users, auth.AuthUser{User: user, Pass: pass})
		}
	}
	return users
}

func parseScript(script Script, rawRules []RawRule) (map[string]C.Matcher, []RawRule, error) {
	var (
		engine        = script.Engine
		path          = script.MainPath
		mainCode      = script.MainCode
		shortcutsCode = script.ShortcutsCode
	)

	if len(shortcutsCode) > 0 && engine != "expr" && engine != "starlark" {
		return nil, nil, fmt.Errorf("invalid script shortcut engine, got %s, want expr or starlark", engine)
	}

	if path != "" {
		if !strings.HasSuffix(path, ".star") {
			return nil, nil, fmt.Errorf("initialized script file failure, script path [%s] invalid", path)
		}
		path = C.Path.Resolve(path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("initialized script file failure, script path invalid: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("initialized script file failure, read file error: %w", err)
		}
		mainCode = string(data)
	}

	if strings.TrimSpace(mainCode) == "" {
		mainCode = `
def main(ctx, metadata):
  return "DIRECT"
`
	} else {
		mainCode = cleanScriptKeywords(mainCode)
	}

	content := mainCode + "\n"

	matcher, err := S.NewMatcher("main", "", mainCode)
	if err != nil {
		return nil, nil, fmt.Errorf("initialized script module failure, %w", err)
	}

	matchers := make(map[string]C.Matcher)
	matchers["main"] = matcher
	for k, v := range shortcutsCode {
		if _, ok := matchers[k]; ok {
			return nil, nil, fmt.Errorf("initialized rule SCRIPT failure, shortcut name [%s] is exist", k)
		}

		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil, fmt.Errorf("initialized rule SCRIPT failure, shortcut [%s] code syntax invalid", k)
		}

		v = strings.ReplaceAll(strings.ReplaceAll(v, "\r", " "), "\n", " ")

		var m C.Matcher
		if engine == "expr" {
			m, err = S.NewExprMatcher(k, v)
		} else {
			m, err = S.NewMatcher(k, "", v)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("initialized script module failure, %w", err)
		}

		matchers[k] = m
		content += v + "\n"
	}

	rpdArr := findRuleProvidersName(content)
	for _, v := range rpdArr {
		rule := fmt.Sprintf("GEOSITE,%s,%s", v, C.ScriptRuleGeoSiteTarget)
		rawRules = append(rawRules, RawRule(rawRule{Line: rule}))
	}

	log.Info().Str("engine", engine).Msg("[Config] initial script module successful")

	return matchers, rawRules, nil
}

func cleanScriptKeywords(code string) string {
	keywords := []string{
		`load\(`, `def resolve_ip\(`, `def in_cidr\(`, `def in_ipset\(`, `def geoip\(`, `def log\(`,
		`def match_provider\(`, `def resolve_process_name\(`, `def resolve_process_path\(`,
	}

	for _, kw := range keywords {
		reg := regexp.MustCompile("(?m)[\r\n]+^.*" + kw + ".*$")
		code = reg.ReplaceAllString(code, "")
	}
	return code
}

func findRuleProvidersName(s string) []string {
	var (
		regxStr = `ctx.rule_providers\[["'](\S+)["']\]\.match|match_provider\(["'](\S+)["']\)`
		regx    = regexp.MustCompile(regxStr)
		arr     = regx.FindAllStringSubmatch(s, -1)
		rpd     []string
	)

	for _, rpdArr := range arr {
		for i, v := range rpdArr {
			if i == 0 || v == "" {
				continue
			}
			rpd = append(rpd, v)
		}
	}

	return lo.Uniq(rpd)
}

func parseMitm(rawMitm RawMitm) (*Mitm, error) {
	var (
		req []C.Rewrite
		res []C.Rewrite
	)

	for _, line := range rawMitm.Rules {
		rule, err := mitm.ParseRewrite(line)
		if err != nil {
			return nil, fmt.Errorf("parse rewrite rule failure: %w", err)
		}

		if rule.RuleType() == C.MitmResponseHeader || rule.RuleType() == C.MitmResponseBody {
			res = append(res, rule)
		} else {
			req = append(req, rule)
		}
	}

	hosts := trie.New[bool]()

	if len(rawMitm.Hosts) != 0 {
		for _, domain := range rawMitm.Hosts {
			_ = hosts.Insert(domain, true)
		}
	}

	_ = hosts.Insert(C.MitmApiHost, true)

	return &Mitm{
		Hosts: hosts,
		Rules: mitm.NewRewriteRules(req, res),
	}, nil
}

func verifyScriptMatcher(config *Config, matchers map[string]C.Matcher) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("test script code panic: %v", r)
		}
	}()

	if len(matchers) == 0 {
		return
	}

	metadata := &C.Metadata{
		Type:    C.SOCKS5,
		NetWork: C.TCP,
		Host:    "www.example.com",
		SrcIP:   netip.MustParseAddr("198.18.0.8"),
		SrcPort: 12345,
		DstPort: 443,
	}

	metadata1 := &C.Metadata{
		Type:    C.TUN,
		NetWork: C.UDP,
		Host:    "8.8.8.8",
		SrcIP:   netip.MustParseAddr("192.168.1.123"),
		SrcPort: 6789,
		DstPort: 2023,
	}

	cases := []*C.Metadata{metadata, metadata1}

	C.BackupScriptState()

	C.GetScriptProxyProviders = func() map[string][]C.Proxy {
		providersMap := make(map[string][]C.Proxy)
		for k, v := range config.Providers {
			providersMap[k] = v.Proxies()
		}
		return providersMap
	}

	C.SetScriptRuleProviders(config.RuleProviders)
	defer C.RestoreScriptState()

	for k, v := range matchers {
		isMain := k == "main"
		for i := range cases {
			if isMain {
				_, err = v.Eval(cases[i])
			} else {
				_, err = v.Match(cases[i])
			}
			if err != nil {
				err = fmt.Errorf("verify script code failed: %w", err)
				return
			}
		}
	}

	return
}
