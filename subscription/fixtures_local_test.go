package subscription

import (
	"os"
	"path/filepath"
	"testing"
)

// Private subscription examples stay outside the repository. Set this variable
// locally to validate imports without committing credentials or server names.
func TestLocalSubscriptionFixtures(t *testing.T) {
	dir := os.Getenv("GLIDER_SUBSCRIPTION_FIXTURES")
	if dir == "" {
		t.Skip("private local fixtures not configured")
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal("cannot read fixture directory")
	}
	for i, f := range files {
		if f.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal("cannot read fixture")
		}
		ns, skip, err := Parse(body)
		if err != nil {
			t.Fatalf("fixture %d parse failed: %v", i+1, err)
		}
		defs := make([]string, 0, len(ns))
		counts := map[string]int{}
		for _, n := range ns {
			defs = append(defs, n.Forward)
			counts[nodeProtocol(n.Forward)]++
		}
		if err := Validate(defs); err != nil {
			t.Fatalf("fixture %d forwarder validation failed", i+1)
		}
		t.Logf("fixture %d: imported=%d skipped=%d protocols=%v", i+1, len(ns), skip, counts)
	}
}
