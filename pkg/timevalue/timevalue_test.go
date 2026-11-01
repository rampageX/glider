package timevalue

import (
	"testing"
	"time"
)

func TestParseUnitsAndCompatibility(t *testing.T) {
	for _, c := range []struct {
		s          string
		unit, want time.Duration
	}{
		{"60", time.Second, time.Minute}, {"2m", time.Second, 2 * time.Minute}, {"2h", time.Second, 2 * time.Hour},
		{"3d", time.Second, 72 * time.Hour}, {"2w", time.Second, 336 * time.Hour}, {"1w2d3h4m5s", time.Second, 219*time.Hour + 4*time.Minute + 5*time.Second},
		{"1.5h", time.Second, 90 * time.Minute}, {"0", time.Second, 0}, {"-1", time.Second, -time.Second},
		{"1000", time.Millisecond, time.Second}, {"12", time.Minute, 12 * time.Minute}, {"7", 24 * time.Hour, 168 * time.Hour},
		{"250ms", time.Second, 250 * time.Millisecond}, {"1m30s", time.Second, 90 * time.Second},
	} {
		got, err := Parse(c.s, c.unit)
		if err != nil || got != c.want {
			t.Fatalf("%s: got %v want %v err=%v", c.s, got, c.want, err)
		}
	}
	for _, s := range []string{"", "1q", "1h junk", "NaN", "1e9s", "999999999999999999999999w", "1s2", "0.1ns"} {
		if _, err := Parse(s, time.Second); err == nil {
			t.Fatalf("accepted invalid %q", s)
		}
	}
	if _, err := Int("500ms", time.Second); err == nil {
		t.Fatal("fractional second silently truncated")
	}
	if n, err := Int("1s", time.Millisecond); err != nil || n != 1000 {
		t.Fatal("millisecond option conversion failed")
	}
}
