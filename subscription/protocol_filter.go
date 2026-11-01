package subscription

import (
	"errors"
	"net/url"
	"strings"
)

func (c *Config) normalizeProtocols() error {
	var normalized []string
	seen := map[string]bool{}
	for _, entry := range c.Protocols {
		for _, item := range strings.Split(entry, ",") {
			protocol := strings.ToLower(strings.TrimSpace(item))
			if protocol == "" {
				continue
			}
			switch protocol {
			case "ss", "ssr", "anytls", "trojan", "vless":
			default:
				return errors.New("invalid subscription-protocol: use ss, ssr, anytls, trojan or vless")
			}
			if !seen[protocol] {
				normalized = append(normalized, protocol)
				seen[protocol] = true
			}
		}
	}
	c.Protocols = normalized
	return nil
}

// Filter applies the region/name filters and protocol allowlist together.
// It does not mutate raw shared caches, so instances may select differently.
func (c Config) Filter(nodes []Node) []string {
	return c.filterProtocols(Filter(nodes, c.Matches, c.Excludes))
}
func (c Config) filterProtocols(defs []string) []string {
	if len(c.Protocols) == 0 {
		return defs
	}
	allowed := map[string]bool{}
	for _, protocol := range c.Protocols {
		allowed[protocol] = true
	}
	var out []string
	for _, def := range defs {
		core, _, _ := strings.Cut(def, "#")
		// The final component identifies the application protocol in a native
		// transport chain. Subscription converters escape literal commas.
		chain := strings.Split(core, ",")
		u, err := url.Parse(chain[len(chain)-1])
		if err == nil && allowed[strings.ToLower(u.Scheme)] {
			out = append(out, def)
		}
	}
	return out
}

// ReadSnapshot reapplies the current allowlist before using a previous run's
// snapshot. Changing protocol must never briefly activate excluded nodes.
func (c Config) ReadSnapshot() ([]string, error) {
	if err := c.normalizeProtocols(); err != nil {
		return nil, err
	}
	defs, err := ReadSnapshot(c.Output)
	if err != nil {
		return nil, err
	}
	defs = c.filterProtocols(defs)
	if len(defs) == 0 {
		return nil, errors.New("snapshot has no nodes matching subscription-protocol")
	}
	return defs, nil
}
