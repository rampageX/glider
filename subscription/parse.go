package subscription

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nadoo/glider/proxy/ss/cipher"
	"github.com/nadoo/glider/proxy/ssr"
)

type Node struct{ Forward, Name, Group string }

var errUnsupported = errors.New("unsupported node")

func decode(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, e := enc.DecodeString(s); e == nil {
			return string(b), nil
		}
	}
	return "", errors.New("invalid base64")
}

// Parse rejects an entire malformed input. Recognized but unsupported nodes are
// skipped explicitly, not silently converted to a different transport.
func Parse(body []byte) (nodes []Node, skipped int, err error) {
	s := cleanLine(string(body))
	if !looksPlain(s) {
		s, err = decode(s)
		if err != nil {
			return nil, 0, fmt.Errorf("stage=envelope reason=%s body=%s", parseReason(err), bodyKind(body))
		}
	}
	if !utf8.ValidString(s) {
		return nil, 0, errors.New("invalid UTF-8")
	}
	for i, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line = cleanLine(line)
		class := lineClass(line)
		if ignoredLine(class) {
			continue
		}
		if strings.HasPrefix(class, "unsupported-") {
			skipped++
			continue
		}
		var n Node
		switch {
		case strings.HasPrefix(line, "ss://"):
			n, err = parseSS(line)
		case strings.HasPrefix(line, "ssr://"):
			n, err = parseSSR(line)
		default:
			err = errors.New("unexpected subscription line")
		}
		if errors.Is(err, errUnsupported) {
			skipped++
			continue
		}
		if err != nil {
			return nil, skipped, fmt.Errorf("stage=node line=%d protocol=%s class=%s reason=%s", i+1, nodeProtocol(line), class, parseReason(err))
		}
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, skipped, errors.New("no supported nodes")
	}
	return nodes, skipped, nil
}
func endpoint(s string) error {
	h, p, e := net.SplitHostPort(s)
	if e != nil || h == "" {
		return errors.New("invalid endpoint")
	}
	n, e := strconv.Atoi(p)
	if e != nil || n < 1 || n > 65535 {
		return errors.New("invalid port")
	}
	if strings.ContainsAny(h, "/?#@, \t\r\n") {
		return errors.New("invalid host")
	}
	return nil
}
func canonical(scheme, method, pass, host, name string, q url.Values) (Node, error) {
	if err := endpoint(host); err != nil {
		return Node{}, err
	}
	u := url.URL{Scheme: scheme, User: url.UserPassword(method, pass), Host: host, RawQuery: q.Encode()}
	// Encode the options once. QueryEscape protects chain separators in names.
	f := u.String() + "#name=" + url.QueryEscape(name)
	f = strings.ReplaceAll(f, ",", "%2C")
	return Node{Forward: f, Name: name}, nil
}
func parseSS(s string) (Node, error) {
	raw := strings.TrimPrefix(s, "ss://")
	name := ""
	if a, b, ok := strings.Cut(raw, "#"); ok {
		raw = a
		var e error
		name, e = url.PathUnescape(b)
		if e != nil {
			return Node{}, atStage("ss-name-percent", e)
		}
	}
	// Query belongs to SIP002, never to a base64 payload.
	query := ""
	if a, b, ok := strings.Cut(raw, "?"); ok {
		raw = a
		query = b
	}
	q, e := url.ParseQuery(query)
	if e != nil {
		return Node{}, atStage("ss-query", e)
	}
	if len(q) > 0 {
		return Node{}, errUnsupported
	} // plugin and unknown extensions
	raw = strings.TrimSuffix(raw, "/")
	var user, host string
	at := strings.LastIndexByte(raw, '@')
	if at < 0 {
		decoded, e := decode(raw)
		if e != nil {
			return Node{}, atStage("ss-whole-base64", e)
		}
		at = strings.LastIndexByte(decoded, '@')
		if at < 0 {
			return Node{}, errors.New("missing endpoint")
		}
		user, host = decoded[:at], decoded[at+1:]
		// Whole-link base64 contains literal credentials; do not percent decode it.
	} else {
		user, host = raw[:at], raw[at+1:]
		if !strings.Contains(user, ":") && !strings.Contains(strings.ToLower(user), "%3a") {
			user, e = decode(user)
		} else {
			user, e = url.PathUnescape(user)
		}
		if e != nil {
			return Node{}, atStage("ss-userinfo", e)
		}
	}
	method, pass, ok := strings.Cut(user, ":")
	if !ok || method == "" {
		return Node{}, errors.New("missing credentials")
	}
	if _, e = cipher.PickCipher(method, nil, pass); e != nil {
		return Node{}, errUnsupported
	}
	return canonical("ss", method, pass, host, name, nil)
}
func parseSSR(s string) (Node, error) {
	decoded, e := decode(strings.TrimPrefix(s, "ssr://"))
	if e != nil {
		return Node{}, atStage("ssr-envelope-base64", e)
	}
	core, query, _ := strings.Cut(decoded, "/?")
	// Split from the right to preserve IPv6 host colons.
	parts := make([]string, 6)
	for i := 5; i > 0; i-- {
		j := strings.LastIndexByte(core, ':')
		if j < 0 {
			return Node{}, errors.New("invalid SSR")
		}
		parts[i] = core[j+1:]
		core = core[:j]
	}
	parts[0] = core
	port, e := strconv.Atoi(parts[1])
	if e != nil {
		return Node{}, atStage("ssr-port", e)
	}
	pass, e := decode(parts[5])
	if e != nil {
		return Node{}, atStage("ssr-password-base64", e)
	}
	// SSR base64 parameters use '+' literally, unlike form-encoded queries.
	q, e := url.ParseQuery(strings.ReplaceAll(query, "+", "%2B"))
	if e != nil {
		return Node{}, atStage("ssr-query", e)
	}
	vals := map[string]string{}
	for _, key := range []string{"remarks", "group", "protoparam", "obfsparam"} {
		if v := q.Get(key); v != "" {
			vals[key], e = decode(v)
			if e != nil {
				return Node{}, atStage("ssr-"+key+"-base64", e)
			}
		}
	}
	params := url.Values{"protocol": {parts[2]}, "obfs": {parts[4]}, "protocol_param": {vals["protoparam"]}, "obfs_param": {vals["obfsparam"]}}
	n, e := canonical("ssr", parts[3], pass, net.JoinHostPort(strings.Trim(parts[0], "[]"), strconv.Itoa(port)), vals["remarks"], params)
	if e == nil {
		if _, err := ssr.NewSSR(strings.SplitN(n.Forward, "#", 2)[0], nil); err != nil {
			return Node{}, errUnsupported
		}
	}
	n.Group = vals["group"]
	return n, e
}
func Filter(nodes []Node, matches, excludes []string) []string {
	var out []string
	for _, n := range nodes {
		text := n.Name
		if n.Group != "" {
			text = n.Group + " " + text
		}
		include := len(matches) == 0
		for _, k := range matches {
			if strings.Contains(text, k) {
				include = true
			}
		}
		for _, k := range excludes {
			if k != "" && strings.Contains(text, k) {
				include = false
				break
			}
		}
		if include {
			out = append(out, n.Forward)
		}
	}
	return out
}
