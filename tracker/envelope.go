package tracker

import (
	"encoding/json"
	"regexp"
)

// AdapterProtocol is the envelope version sdd speaks to plugins.
const AdapterProtocol = "sdd.adapter/v1"

// Adapter error codes.
const (
	ErrCodeNotFound          = "not_found"
	ErrCodeUnsupported       = "unsupported"
	ErrCodeAuth              = "auth"
	ErrCodeInvalid           = "invalid"
	ErrCodeInvalidTransition = "invalid_transition"
	ErrCodeRateLimited       = "rate_limited"
	ErrCodeUnavailable       = "unavailable"
)

// AdapterRequest is the one JSON document sdd writes to a plugin's stdin.
type AdapterRequest struct {
	Protocol string          `json:"protocol"`
	Kind     string          `json:"kind"`
	Op       string          `json:"op"`
	Config   json.RawMessage `json:"config,omitempty"`
	Args     json.RawMessage `json:"args,omitempty"`
}

// AdapterResponse is the one JSON document a plugin writes to stdout.
type AdapterResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *AdapterError   `json:"error,omitempty"`
}

// AdapterError is a plugin-reported failure.
type AdapterError struct {
	Code       string          `json:"code"`
	Message    string          `json:"message"`
	RetryAfter int             `json:"retry_after,omitempty"`
	Allowed    []string        `json:"allowed,omitempty"` // for invalid_transition
	Detail     json.RawMessage `json:"detail,omitempty"`
}

func (e *AdapterError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Retryable reports whether a failed write should stay in the outbox.
func (e *AdapterError) Retryable() bool {
	switch e.Code {
	case ErrCodeRateLimited, ErrCodeUnavailable, ErrCodeAuth:
		return true
	}
	return false
}

var backendNameRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidBackendName reports whether name may be resolved as sdd-tracker-<name>.
func ValidBackendName(name string) bool { return backendNameRE.MatchString(name) }
