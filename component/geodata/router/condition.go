package router

import (
	"fmt"
	"strings"

	"github.com/yaling888/quirktiva/component/geodata/strmatcher"
)

var matcherTypeMap = map[Domain_Type]strmatcher.Type{
	Domain_Plain:  strmatcher.Substr,
	Domain_Regex:  strmatcher.Regex,
	Domain_Domain: strmatcher.Domain,
	Domain_Full:   strmatcher.Full,
}

func domainToMatcher(domain *Domain) (strmatcher.Matcher, error) {
	matcherType, f := matcherTypeMap[domain.GetType()]
	if !f {
		return nil, fmt.Errorf("unsupported domain type %v", domain.GetType())
	}

	matcher, err := matcherType.New(domain.GetValue())
	if err != nil {
		return nil, fmt.Errorf("failed to create domain matcher, base error: %w", err)
	}

	return matcher, nil
}

type DomainMatcher struct {
	matchers strmatcher.IndexMatcher
	not      bool
}

func NewMphMatcherGroup(domains []*Domain, not bool) (*DomainMatcher, error) {
	g := strmatcher.NewMphMatcherGroup()
	for _, d := range domains {
		matcherType, f := matcherTypeMap[d.GetType()]
		if !f {
			return nil, fmt.Errorf("unsupported domain type %v", d.GetType())
		}
		_, err := g.AddPattern(d.GetValue(), matcherType)
		if err != nil {
			return nil, err
		}
	}
	g.Build()
	return &DomainMatcher{
		matchers: g,
		not:      not,
	}, nil
}

// NewDomainMatcher new domain matcher.
func NewDomainMatcher(domains []*Domain, not bool) (*DomainMatcher, error) {
	g := new(strmatcher.MatcherGroup)
	for _, d := range domains {
		m, err := domainToMatcher(d)
		if err != nil {
			return nil, err
		}
		g.Add(m)
	}

	return &DomainMatcher{
		matchers: g,
		not:      not,
	}, nil
}

func (m *DomainMatcher) ApplyDomain(domain string) bool {
	if m.not {
		return len(m.matchers.Match(strings.ToLower(domain))) == 0
	}
	return len(m.matchers.Match(strings.ToLower(domain))) > 0
}
