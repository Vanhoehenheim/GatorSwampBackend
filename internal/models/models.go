// Package models contains all the data models for the application
package models

// StatusResponse is a generic struct for simple success/error messages.
// It can be used by actor responses or HTTP handlers.
type StatusResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"` // Optional error detail
}
