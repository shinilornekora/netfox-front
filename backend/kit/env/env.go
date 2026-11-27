// Package env — чтение настроек из окружения с накоплением ошибок:
// сервис сообщает обо всех неверных переменных сразу, а не по одной.
package env

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Reader struct{ errs []error }

func (r *Reader) Str(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

func (r *Reader) Must(name string) string {
	v := r.Str(name, "")
	if v == "" {
		r.errs = append(r.errs, fmt.Errorf("%s is required", name))
	}
	return v
}

func (r *Reader) Dur(name string, def time.Duration) time.Duration {
	raw := r.Str(name, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %w", name, err))
		return def
	}
	return d
}

func (r *Reader) Int(name string, def int) int {
	raw := r.Str(name, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %w", name, err))
		return def
	}
	return n
}

func (r *Reader) List(name string, def ...string) []string {
	raw := r.Str(name, "")
	if raw == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Err — все накопленные ошибки разом.
func (r *Reader) Err() error { return errors.Join(r.errs...) }
