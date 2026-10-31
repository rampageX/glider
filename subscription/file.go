package subscription

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// atomicWrite never exposes a partial file. Temporary and final files are 0600.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".subscription-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func Render(defs []string) []byte {
	var b bytes.Buffer
	b.WriteString("# Glider managed subscription snapshot; credentials; chmod 600; do not edit/include\n")
	for _, s := range defs {
		b.WriteString("forward=" + s + "\n")
	}
	return b.Bytes()
}

// ReadSnapshot deliberately accepts forward entries only, never general config.
func ReadSnapshot(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) > 32<<20 {
		return nil, errors.New("snapshot too large")
	}
	var defs []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, ok := strings.CutPrefix(line, "forward=")
		if !ok || s == "" {
			return nil, errors.New("invalid snapshot")
		}
		defs = append(defs, s)
	}
	if len(defs) == 0 {
		return nil, errors.New("empty snapshot")
	}
	return defs, nil
}
