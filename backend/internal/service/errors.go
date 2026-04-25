package service

import (
	"fmt"

	"arenea/backend/internal/domain"
)

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrValidation, fmt.Sprintf(format, args...))
}

func conflictError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrConflict, fmt.Sprintf(format, args...))
}
