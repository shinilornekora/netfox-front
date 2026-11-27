// Package problem — ошибки в формате RFC 7807 (application/problem+json).
//
// Стандартный формат, а не самодельный: его понимают HTTP-клиенты и
// инструменты, а поле type служит стабильным машинным кодом ошибки.
package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Problem — ошибка прикладного уровня со статусом и кодом.
type Problem struct {
	Status int               `json:"status"`
	Code   string            `json:"code"`
	Title  string            `json:"title"`
	Detail string            `json:"detail,omitempty"`
	Fields map[string]string `json:"errors,omitempty"`
	Trace  string            `json:"trace_id,omitempty"`
	cause  error
}

func (p *Problem) Error() string {
	if p.cause != nil {
		return fmt.Sprintf("%d %s: %s: %v", p.Status, p.Code, p.Title, p.cause)
	}
	return fmt.Sprintf("%d %s: %s", p.Status, p.Code, p.Title)
}

func (p *Problem) Unwrap() error { return p.cause }

// With добавляет причину (для журнала, не для клиента).
func (p *Problem) With(err error) *Problem { c := *p; c.cause = err; return &c }

func New(status int, code, title string) *Problem {
	return &Problem{Status: status, Code: code, Title: title}
}

func Invalid(fields map[string]string) *Problem {
	return &Problem{Status: http.StatusUnprocessableEntity, Code: "invalid", Title: "Проверьте данные формы", Fields: fields}
}

func NotFound(what string) *Problem {
	return New(http.StatusNotFound, "not_found", what+": не найдено")
}

func Conflict(code, title string) *Problem { return New(http.StatusConflict, code, title) }

func Forbidden(title string) *Problem { return New(http.StatusForbidden, "forbidden", title) }

func Unauthenticated() *Problem {
	return New(http.StatusUnauthorized, "unauthenticated", "Нужно войти в аккаунт")
}

func Unavailable(code, title string) *Problem {
	return New(http.StatusServiceUnavailable, code, title)
}

// From приводит любую ошибку к Problem; неизвестные — 500 без подробностей.
func From(err error) *Problem {
	var p *Problem
	if errors.As(err, &p) {
		return p
	}
	return &Problem{Status: 500, Code: "internal", Title: "Что-то пошло не так, попробуйте позже", cause: err}
}

// Is проверяет код ошибки.
func Is(err error, code string) bool {
	var p *Problem
	return errors.As(err, &p) && p.Code == code
}

// Write отправляет problem+json.
func Write(w http.ResponseWriter, p *Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// Fields — накопитель ошибок валидации.
type Fields map[string]string

func (f Fields) Set(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

func (f Fields) Err() error {
	if len(f) == 0 {
		return nil
	}
	return Invalid(f)
}
