package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// env reads typed environment variables and accumulates every problem, so a
// misconfigured deployment reports all bad keys at once instead of failing
// on the first one.
type env struct {
	errs []error
}

func (e *env) lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (e *env) str(key, fallback string) string {
	if v, ok := e.lookup(key); ok {
		return v
	}
	return fallback
}

func (e *env) required(key string) string {
	v, ok := e.lookup(key)
	if !ok {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (e *env) int(key string, fallback int) int {
	v, ok := e.lookup(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s must be an integer, got %q", key, v))
		return fallback
	}
	return n
}

func (e *env) duration(key string, fallback time.Duration) time.Duration {
	v, ok := e.lookup(key)
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s must be a duration like 15s, 5m, 1h; got %q", key, v))
		return fallback
	}
	return d
}

func (e *env) bool(key string, fallback bool) bool {
	v, ok := e.lookup(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	e.errs = append(e.errs, fmt.Errorf("%s must be a boolean, got %q", key, v))
	return fallback
}

func (e *env) list(key, fallback string) []string {
	raw := e.str(key, fallback)
	parts := strings.Split(raw, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (e *env) check(ok bool, format string, args ...any) {
	if !ok {
		e.errs = append(e.errs, fmt.Errorf(format, args...))
	}
}
