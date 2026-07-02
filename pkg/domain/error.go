package domain

import "errors"

// ErrorKind determines the severity and category of a domain execution error.
type ErrorKind string

const (
	// ErrTransient represents temporary failures that can be retried.
	ErrTransient ErrorKind = "TRANSIENT"
	// ErrPermanent represents fatal logic errors requiring configuration change.
	ErrPermanent ErrorKind = "PERMANENT"
	// ErrSandboxBlock represents sandbox policy violations.
	ErrSandboxBlock ErrorKind = "SANDBOX_VIOLATION"
	// ErrValidation represents feature verification failures.
	ErrValidation ErrorKind = "VALIDATION_FAILURE"
)

// Sentinel errors for standard failure classification.
var (
	ErrTaskNotFound          = errors.New("task not found")
	ErrVersionConflict       = errors.New("optimistic concurrency version conflict")
	ErrSandboxViolation      = errors.New("sandbox boundary violation")
	ErrBudgetExhausted       = errors.New("LLM token budget exhausted")
	ErrMaxRetriesReached     = errors.New("maximum retries exceeded")
	ErrSecurityVulnerability = errors.New("SAST scan found blocking security vulnerabilities")
	ErrSelfPatchFailed       = errors.New("self-patch build or test suite failed")
	ErrHotReloadFailed       = errors.New("hot-reload handshake failed")
)

// DomainError groups errors for structured tracking and propagation.
type DomainError struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

// Error formats the DomainError as a readable string representation.
func (e *DomainError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap exposes the underlying cause to enable errors.Is/As evaluations.
func (e *DomainError) Unwrap() error {
	return e.Cause
}
