package httputil

import (
	"encoding/json"
	"net/http"
)

// Error codes matching API specification
const (
	ErrCodeBadRequest   = "BAD_REQUEST"
	ErrCodeUnauthorized = "UNAUTHORIZED"
	ErrCodeForbidden    = "FORBIDDEN"
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeConflict     = "CONFLICT"
	ErrCodeInternal     = "INTERNAL_ERROR"
)

// ErrorResponse represents the standard error response format
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and message
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes a JSON response with the given status code
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			// If encoding fails, we can't do much - headers already sent
			// Log would be useful here in production
			return
		}
	}
}

// WriteError writes an error response matching API spec format:
// {"error": {"code": "ERROR_CODE", "message": "Human readable message"}}
func WriteError(w http.ResponseWriter, status int, code string, message string) {
	response := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	WriteJSON(w, status, response)
}

// Common error response helpers

// WriteBadRequest writes a 400 Bad Request error
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteBadRequestWithCode writes a 400 Bad Request error with a custom code
func WriteBadRequestWithCode(w http.ResponseWriter, code string, message string) {
	WriteError(w, http.StatusBadRequest, code, message)
}

// WriteUnauthorized writes a 401 Unauthorized error
func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// WriteUnauthorizedWithCode writes a 401 Unauthorized error with a custom code
func WriteUnauthorizedWithCode(w http.ResponseWriter, code string, message string) {
	WriteError(w, http.StatusUnauthorized, code, message)
}

// WriteForbidden writes a 403 Forbidden error
func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// WriteNotFound writes a 404 Not Found error
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteConflict writes a 409 Conflict error
func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, ErrCodeConflict, message)
}

// WriteInternalError writes a 500 Internal Server Error
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternal, message)
}
