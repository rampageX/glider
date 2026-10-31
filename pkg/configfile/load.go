// Package configfile loads the main configuration once, expanding includes in
// place. conflag still owns flag types and CLI syntax; rule files are unchanged.
package configfile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nadoo/conflag"
)

func Parse(c *conflag.Conflag, args []string, app string) error {
	file := ""
	for i := 0; i < len(args); i++ {
		a := strings.TrimLeft(args[i], "-")
		if strings.HasPrefix(a, "config=") {
			file = strings.TrimPrefix(a, "config=")
		}
		if a == "config" && i+1 < len(args) {
			i++
			file = args[i]
		}
	}
	if file == "" && len(args) == 0 {
		file = strings.TrimSuffix(app, filepath.Ext(app)) + ".conf"
	}
	var expanded []string
	loaded := map[string]bool{}
	if file != "" {
		var err error
		expanded, err = read(file, map[string]bool{}, loaded)
		if err != nil {
			return err
		}
		// Set conflag's config path for ConfDir(), without parsing any values twice.
		expanded = append([]string{"-config=" + file}, expanded...)
	}
	expanded = append(expanded, args...)
	output := ""
	for i, a := range expanded {
		if strings.HasPrefix(a, "-subscription-output=") {
			output = strings.TrimPrefix(a, "-subscription-output=")
		}
		if a == "-subscription-output" && i+1 < len(expanded) {
			output = expanded[i+1]
		}
	}
	if output != "" {
		if !filepath.IsAbs(output) {
			output = filepath.Join(filepath.Dir(file), output)
		}
		abs, _ := filepath.Abs(output)
		if loaded[abs] {
			return fmt.Errorf("subscription-output must not also be an include")
		}
	}
	return c.FlagSet.Parse(expanded)
}

func read(file string, stack map[string]bool, loaded map[string]bool) ([]string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	if stack[abs] {
		return nil, fmt.Errorf("include cycle")
	}
	if len(stack) >= 32 {
		return nil, fmt.Errorf("include depth exceeds 32")
	}
	stack[abs] = true
	loaded[abs] = true
	defer delete(stack, abs)
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 4<<20)
	for s.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(s.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Preserve conflag's {$VAR} syntax, without recursively expanding values.
		for start := 0; ; {
			i := strings.Index(line[start:], "{$")
			if i < 0 {
				break
			}
			i += start
			j := strings.IndexByte(line[i:], '}')
			if j < 0 {
				break
			}
			j += i
			v := os.Getenv(line[i+2 : j])
			line = line[:i] + v + line[j+1:]
			start = i + len(v)
		}
		key, val, has := strings.Cut(line, "=")
		if strings.TrimSpace(key) == "include" {
			if !has || strings.TrimSpace(val) == "" {
				return nil, fmt.Errorf("empty include")
			}
			target := strings.TrimSpace(val)
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(abs), target)
			}
			nested, e := read(target, stack, loaded)
			if e != nil {
				return nil, e
			}
			out = append(out, nested...)
		} else {
			out = append(out, "-"+line)
		}
	}
	return out, s.Err()
}
