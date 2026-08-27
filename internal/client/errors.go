package client

import (
	"encoding/json"
	"errors"
	"fmt"
)

// APIError is a safe representation of a Dokploy API failure.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Operation  string
}

func (e *APIError) Error() string {
	if e.Operation == "" {
		if e.Code != "" && e.Message != "" {
			return fmt.Sprintf("%s: %s", e.Code, e.Message)
		}
		if e.Code != "" {
			return e.Code
		}
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s: %s", e.Operation, e.Code, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Operation, e.Code)
	}
	return fmt.Sprintf("%s: HTTP %d", e.Operation, e.StatusCode)
}

func decodeError(operation string, status int, body []byte) error {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return &APIError{StatusCode: status, Operation: operation}
	}
	return &APIError{StatusCode: status, Code: payload.Code, Message: payload.Message, Operation: operation}
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.Code == "NOT_FOUND")
}

func IsTransient(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}
