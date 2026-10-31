package subscription

import (
	"strconv"
	"strings"
)

// lineClass returns fixed labels only. Unknown content remains a fatal parse
// error; only recognized comments/headers and named unsupported schemes skip.
func cleanLine(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "\ufeff"))
}
func lineClass(line string) string {
	line = cleanLine(line)
	if line == "" {
		return "empty"
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
		return "comment"
	}
	if strings.HasPrefix(line, "MAX=") {
		if _, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "MAX=")), 10, 64); err != nil {
			return "invalid-metadata-max"
		}
		return "metadata-max"
	}
	if strings.HasPrefix(line, "STATUS=") {
		return "metadata-status"
	}
	if strings.HasPrefix(line, "REMARKS=") {
		return "metadata-remarks"
	}
	scheme, _, ok := strings.Cut(line, "://")
	if ok {
		switch scheme {
		case "ss", "ssr":
			return scheme
		case "vmess", "vless", "trojan", "hysteria", "hysteria2", "hy2", "tuic", "wireguard", "snell":
			return "unsupported-" + scheme
		}
		return "unknown-scheme"
	}
	return "unknown-text"
}
func ignoredLine(class string) bool {
	return class == "empty" || class == "comment" || class == "metadata-max" || class == "metadata-status" || class == "metadata-remarks"
}
func looksPlain(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		class := lineClass(line)
		if class == "ss" || class == "ssr" || strings.HasPrefix(class, "metadata-") || class == "comment" || strings.HasPrefix(class, "unsupported-") {
			return true
		}
	}
	return false
}
