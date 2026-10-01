// Package config reads and validates each binary's environment.
package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type problems []string

func (p problems) Error() string {
	return "invalid configuration:\n  - " + strings.Join(p, "\n  - ")
}

type reader struct {
	getenv   func(string) string
	problems problems
}

func (r *reader) problemf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func (r *reader) err() error {
	if len(r.problems) == 0 {
		return nil
	}
	return r.problems
}

func (r *reader) optional(key string) string {
	return strings.TrimSpace(r.getenv(key))
}

func (r *reader) required(key string) string {
	v := r.optional(key)
	if v == "" {
		r.problemf("%s is required", key)
	}
	return v
}

func (r *reader) hostPort(key string) string {
	v := r.required(key)
	if _, _, err := net.SplitHostPort(v); v != "" && err != nil {
		r.problemf("%s must be host:port (got %q)", key, v)
	}
	return v
}

func (r *reader) port(key string, fallback int) int {
	raw := r.optional(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		r.problemf("%s must be a port, 1-65535 (got %q)", key, raw)
		return fallback
	}
	return n
}
