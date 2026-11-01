// Package timevalue parses configuration durations while preserving each
// option's historical unit for values without a suffix.
package timevalue

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nadoo/conflag"
)

var number = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)
var component = regexp.MustCompile(`^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(ns|us|µs|μs|ms|s|m|h|d|w)`)
var units = map[string]time.Duration{"ns": time.Nanosecond, "us": time.Microsecond, "µs": time.Microsecond, "μs": time.Microsecond, "ms": time.Millisecond, "s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}

// Parse accepts bare values in bareUnit and suffixed/compound durations.
// Rational arithmetic checks overflow before converting to nanoseconds.
func Parse(raw string, bareUnit time.Duration) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	negative := false
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" || bareUnit <= 0 {
		return 0, errors.New("invalid duration")
	}
	total := new(big.Rat)
	add := func(n string, u time.Duration) bool {
		v, ok := new(big.Rat).SetString(n)
		if !ok {
			return false
		}
		v.Mul(v, new(big.Rat).SetInt64(int64(u)))
		total.Add(total, v)
		return true
	}
	if number.MatchString(s) {
		if !add(s, bareUnit) {
			return 0, errors.New("invalid duration")
		}
	} else {
		for s != "" {
			m := component.FindStringSubmatch(s)
			if m == nil || !add(m[1], units[m[2]]) {
				return 0, errors.New("invalid duration unit or value")
			}
			s = s[len(m[0]):]
		}
	}
	if negative {
		total.Neg(total)
	}
	if !total.IsInt() || !total.Num().IsInt64() {
		return 0, errors.New("duration overflows or is below nanosecond precision")
	}
	return time.Duration(total.Num().Int64()), nil
}
func Int(raw string, unit time.Duration) (int, error) {
	d, err := Parse(raw, unit)
	if err != nil {
		return 0, err
	}
	if d%unit != 0 {
		return 0, errors.New("duration is below this option's precision")
	}
	n := int64(d / unit)
	if strconv.IntSize == 32 && (n > 1<<31-1 || n < -1<<31) {
		return 0, errors.New("duration overflows integer")
	}
	return int(n), nil
}

type value struct {
	target *int
	unit   time.Duration
}

func (v *value) String() string {
	if v.target == nil {
		return "0"
	}
	return strconv.Itoa(*v.target)
}
func (v *value) Set(raw string) error {
	n, err := Int(raw, v.unit)
	if err == nil {
		*v.target = n
	}
	return err
}
func Var(f *conflag.Conflag, p *int, name string, initial int, unit time.Duration, usage string) {
	*p = initial
	bare := "seconds"
	suffixes := "s/m/h/d/w"
	if unit == time.Millisecond {
		bare = "milliseconds"
		suffixes = "ms/s/m/h/d/w"
	}
	f.Var(&value{p, unit}, name, usage+"; bare: "+bare+"; suffixes: "+suffixes)
}
func SecondsVar(f *conflag.Conflag, p *int, name string, initial int, usage string) {
	Var(f, p, name, initial, time.Second, usage)
}
func MillisecondsVar(f *conflag.Conflag, p *int, name string, initial int, usage string) {
	Var(f, p, name, initial, time.Millisecond, usage)
}
