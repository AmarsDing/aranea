package domain

import "errors"

var (
	ErrValidation   = errors.New("validation error")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInternal     = errors.New("internal error")

	// L1 working-memory specific errors. They wrap the generic ones above so
	// the HTTP transport can map them to 422 / 409 without a separate switch
	// while still letting business code distinguish each cause.
	// See aranea/docs/13 memory-L1-working.md §5.2 and §9.
	ErrL1Overflow        = errors.New("l1 overflow")
	ErrFieldTooLarge     = errors.New("l1 field too large")
	ErrRevisionConflict  = errors.New("l1 revision conflict")
	ErrTaskNotWritable   = errors.New("l1 task not writable")
	ErrInvalidFieldPath  = errors.New("l1 invalid field path")
	ErrInvalidFieldValue = errors.New("l1 invalid field value")
)
