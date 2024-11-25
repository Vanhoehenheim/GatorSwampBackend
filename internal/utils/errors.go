package utils

type AppError struct {
	Code    string
	Message string
	Origin  error // Original error that caused this error, if any
}

func (appErr *AppError) Error() string {
	if appErr.Origin != nil {
		return appErr.Message + ": " + appErr.Origin.Error()
	}
	return appErr.Message
}

// Standard error codes for the application
const (
	ErrNotFound     = "NOT_FOUND"
	ErrDuplicate    = "DUPLICATE"
	ErrInvalidInput = "INVALID_INPUT"
	ErrUnauthorized = "UNAUTHORIZED"
)

func NewAppError(code string, message string, originalErr error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Origin:  originalErr,
	}
}
