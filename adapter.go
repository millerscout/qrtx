package qrtx

import (
	"fmt"

	"github.com/millerscout/qrtx/tracker"
)

// AdapterProtocol is the envelope version qrtx speaks to plugins.
const AdapterProtocol = tracker.AdapterProtocol

// Adapter error codes.
const (
	ErrCodeNotFound          = tracker.ErrCodeNotFound
	ErrCodeUnsupported       = tracker.ErrCodeUnsupported
	ErrCodeAuth              = tracker.ErrCodeAuth
	ErrCodeInvalid           = tracker.ErrCodeInvalid
	ErrCodeInvalidTransition = tracker.ErrCodeInvalidTransition
	ErrCodeRateLimited       = tracker.ErrCodeRateLimited
	ErrCodeUnavailable       = tracker.ErrCodeUnavailable
)

type (
	AdapterRequest  = tracker.AdapterRequest
	AdapterResponse = tracker.AdapterResponse
	AdapterError    = tracker.AdapterError
)

// ValidBackendName reports whether name may be resolved as qrtx-tracker-<name>.
func ValidBackendName(name string) bool { return tracker.ValidBackendName(name) }

func checkBackendName(name string) error {
	if !ValidBackendName(name) {
		return fmt.Errorf("tracker backend %q is invalid: use lowercase letters, digits and dashes (resolved as qrtx-tracker-<name> on PATH)", name)
	}
	return nil
}
