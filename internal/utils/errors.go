package utils

import "fmt"

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
	// Resource errors
	ErrNotFound     = "NOT_FOUND"
	ErrDuplicate    = "DUPLICATE"
	ErrInvalidInput = "INVALID_INPUT"

	// Authentication/Authorization errors
	ErrUnauthorized = "UNAUTHORIZED"
	ErrForbidden    = "FORBIDDEN" // User is authenticated but doesn't have permission
	ErrInvalidToken = "INVALID_TOKEN"

	// User-specific errors
	ErrUserNotFound       = "USER_NOT_FOUND"
	ErrUserAlreadyExists  = "USER_ALREADY_EXISTS"
	ErrInsufficientKarma  = "INSUFFICIENT_KARMA"
	ErrInvalidCredentials = "INVALID_CREDENTIALS"

	// Subreddit-specific errors
	ErrSubredditNotFound      = "SUBREDDIT_NOT_FOUND"
	ErrSubredditExists        = "SUBREDDIT_EXISTS"
	ErrNotSubredditMember     = "NOT_SUBREDDIT_MEMBER"
	ErrAlreadySubredditMember = "ALREADY_SUBREDDIT_MEMBER"

	// Actor communication errors
	ErrActorTimeout    = "ACTOR_TIMEOUT"
	ErrActorNotFound   = "ACTOR_NOT_FOUND"
	ErrMessageRejected = "MESSAGE_REJECTED"

	// Rate limiting
	ErrTooManyRequests = "TOO_MANY_REQUESTS"

	ErrDatabase = "database_error"
)

// Error creation helper functions
func NewAppError(code string, message string, originalErr error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Origin:  originalErr,
	}
}

// Specific error creators for common cases
func NewUserNotFoundError(userId string) *AppError {
	return &AppError{
		Code:    ErrUserNotFound,
		Message: "User not found: " + userId,
	}
}

func NewUnauthorizedError(reason string) *AppError {
	return &AppError{
		Code:    ErrUnauthorized,
		Message: "Unauthorized: " + reason,
	}
}

func NewSubredditNotFoundError(subredditName string) *AppError {
	return &AppError{
		Code:    ErrSubredditNotFound,
		Message: "Subreddit not found: " + subredditName,
	}
}

func NewInsufficientKarmaError(required int, actual int) *AppError {
	return &AppError{
		Code:    ErrInsufficientKarma,
		Message: fmt.Sprintf("Insufficient karma. Required: %d, Actual: %d", required, actual),
	}
}

func NewActorTimeoutError(actorName string) *AppError {
	return &AppError{
		Code:    ErrActorTimeout,
		Message: "Actor communication timeout: " + actorName,
	}
}

// Helper method to check if an error is of a specific type
func IsErrorCode(err error, code string) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code == code
	}
	return false
}

// Helper method to check if an error is related to authentication
func IsAuthError(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code == ErrUnauthorized ||
			appErr.Code == ErrForbidden ||
			appErr.Code == ErrInvalidToken
	}
	return false
}

// AppErrorToHTTPStatus converts an AppError code to an HTTP status code.
func AppErrorToHTTPStatus(errorCode string) int {
	switch errorCode {
	case ErrNotFound, ErrUserNotFound, ErrSubredditNotFound, ErrActorNotFound:
		return 404 // http.StatusNotFound
	case ErrInvalidInput, ErrInvalidCredentials:
		return 400 // http.StatusBadRequest
	case ErrUnauthorized, ErrInvalidToken:
		return 401 // http.StatusUnauthorized
	case ErrForbidden, ErrNotSubredditMember:
		return 403 // http.StatusForbidden
	case ErrDuplicate, ErrUserAlreadyExists, ErrSubredditExists, ErrAlreadySubredditMember:
		return 409 // http.StatusConflict
	case ErrTooManyRequests:
		return 429 // http.StatusTooManyRequests
	case ErrDatabase, ErrActorTimeout, ErrMessageRejected:
		return 500 // http.StatusInternalServerError
	default:
		return 500 // http.StatusInternalServerError for unknown errors
	}
}
