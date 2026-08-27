package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// APIError is a safe representation of a Dokploy API failure.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Operation  string
}

func (e *APIError) Error() string {
	code := redactText(e.Code, "")
	message := redactText(e.Message, "")
	if e.Operation == "" {
		if code != "" && message != "" {
			return fmt.Sprintf("%s: %s", code, message)
		}
		if code != "" {
			return code
		}
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if code != "" && message != "" {
		return fmt.Sprintf("%s: %s: %s", e.Operation, code, message)
	}
	if code != "" {
		return fmt.Sprintf("%s: %s", e.Operation, code)
	}
	return fmt.Sprintf("%s: HTTP %d", e.Operation, e.StatusCode)
}

func decodeError(operation string, status int, body []byte) error {
	return decodeErrorWithSecret(operation, status, body, "")
}

func decodeErrorWithSecret(operation string, status int, body []byte, secret string) error {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return &APIError{StatusCode: status, Operation: operation}
	}
	return &APIError{StatusCode: status, Code: redactText(payload.Code, secret), Message: redactText(payload.Message, secret), Operation: operation}
}

func redactText(text, secret string) string {
	if secret != "" {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	lower := strings.ToLower(text)
	for _, field := range []string{"apikey", "api-key", "password", "token", "secret", "credential", "environment", "buildarg", "buildsecret", "registry"} {
		if strings.Contains(lower, field) {
			return "[REDACTED]"
		}
	}
	return text
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
