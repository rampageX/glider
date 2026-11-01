package subscription

import (
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/nadoo/glider/proxy/vless"
)

// aliasValue rejects contradictory aliases rather than guessing which security
// setting the subscription author intended.
func aliasValue(q url.Values, keys ...string) (string, error) {
	var value string
	for _, key := range keys {
		for _, v := range q[key] {
			if value != "" && v != "" && value != v {
				return "", errors.New("conflicting aliases")
			}
			if v != "" {
				value = v
			}
		}
	}
	return value, nil
}

func parseModern(raw string) (Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, errors.New("invalid node URL")
	}
	if err = endpoint(u.Host); err != nil {
		return Node{}, err
	}
	if u.User == nil || u.User.Username() == "" {
		return Node{}, errors.New("missing credentials")
	}
	if _, ok := u.User.Password(); ok {
		return Node{}, errors.New("unexpected password field")
	}
	if u.Path != "" && u.Path != "/" {
		return Node{}, errUnsupported
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Node{}, errors.New("invalid query")
	}
	allowed := map[string]bool{}
	for _, k := range strings.Fields("sni peer serverName servername insecure allowInsecure skipVerify type security alpn") {
		allowed[k] = true
	}
	if u.Scheme == "vless" {
		for _, k := range strings.Fields("encryption flow fp pbk sid spx mode") {
			allowed[k] = true
		}
	} else if u.Scheme == "trojan" {
		allowed["pcs"] = true
	}
	for k, vs := range q {
		if !allowed[k] {
			return Node{}, errUnsupported
		}
		if len(vs) != 1 {
			return Node{}, errors.New("duplicate parameter")
		}
	}
	if t := q.Get("type"); t != "" && t != "tcp" && t != "raw" {
		return Node{}, errUnsupported
	}
	security := q.Get("security")
	if u.Scheme != "vless" && security != "" && security != "tls" {
		return Node{}, errUnsupported
	}
	name, err := aliasValue(q, "serverName", "servername", "sni", "peer")
	if err != nil {
		return Node{}, err
	}
	out := url.Values{}
	if name != "" {
		out.Set("serverName", name)
	}
	var insecure *bool
	for _, key := range []string{"insecure", "allowInsecure", "skipVerify"} {
		if v, ok := q[key]; ok {
			var b bool
			switch strings.ToLower(v[0]) {
			case "1", "true":
				b = true
			case "0", "false":
			default:
				return Node{}, errors.New("invalid boolean")
			}
			if insecure != nil && *insecure != b {
				return Node{}, errors.New("conflicting verification aliases")
			}
			insecure = &b
		}
	}
	if insecure != nil && *insecure {
		out.Set("skipVerify", "true")
	}
	if a := q.Get("alpn"); a != "" {
		// Trojan/AnyTLS currently do not expose ALPN: never silently discard it.
		if u.Scheme != "vless" {
			return Node{}, errUnsupported
		}
		out.Set("alpn", a)
	}
	if u.Scheme == "trojan" && q.Get("pcs") != "" {
		pin, err := hex.DecodeString(strings.ReplaceAll(q.Get("pcs"), ":", ""))
		if err != nil || len(pin) != 32 {
			return Node{}, errors.New("invalid certificate pin")
		}
		out.Set("pcs", q.Get("pcs"))
	}
	if u.Scheme == "vless" {
		if e := q.Get("encryption"); e != "" && e != "none" {
			return Node{}, errUnsupported
		}
		if security == "" {
			security = "none"
		}
		if security != "none" && security != "tls" && security != "reality" {
			return Node{}, errUnsupported
		}
		out.Set("security", security)
		for _, k := range []string{"flow", "fp", "pbk", "sid", "spx"} {
			if q.Get(k) != "" {
				out.Set(k, q.Get(k))
			}
		}
		// mode is an XHTTP setting. Some exporters emit it for TCP too; it
		// does not change the explicitly selected TCP transport.
	}
	nodeName := u.Fragment
	u.Path, u.RawPath, u.RawQuery, u.Fragment, u.RawFragment = "", "", out.Encode(), "", ""
	if u.Scheme == "vless" {
		if err := vless.ValidateClientURL(u.String()); err != nil {
			return Node{}, errUnsupported
		}
	}
	forward := strings.ReplaceAll(u.String(), ",", "%2C") + "#name=" + url.QueryEscape(nodeName)
	return Node{Forward: forward, Name: nodeName}, nil
}
