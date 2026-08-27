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
	return redactSensitiveValues(text)
}

func redactSensitiveValues(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		valueStart, ok := sensitiveValueStart(text, i)
		if !ok {
			out.WriteByte(text[i])
			i++
			continue
		}
		out.WriteString(text[i:valueStart])
		if valueStart < len(text) && (text[valueStart] == '\'' || text[valueStart] == '"') {
			quote := text[valueStart]
			end := quotedValueEnd(text, valueStart+1, quote)
			out.WriteByte(quote)
			out.WriteString("[REDACTED]")
			if end < len(text) {
				out.WriteByte(quote)
				end++
			}
			i = end
			continue
		}
		end := unquotedValueEnd(text, valueStart)
		out.WriteString("[REDACTED]")
		i = end
	}
	return out.String()
}

func sensitiveValueStart(text string, start int) (int, bool) {
	if start > 0 && (isWord(text[start-1]) || text[start-1] == '_') {
		return 0, false
	}
	i := start
	if i < len(text) && (text[i] == '\'' || text[i] == '"') {
		quote := text[i]
		i++
		keyStart := i
		for i < len(text) && text[i] != quote {
			i++
		}
		if i == len(text) || !sensitiveKey(text[keyStart:i]) {
			return 0, false
		}
		i++
	} else {
		keyStart := i
		for i < len(text) && (isWord(text[i]) || text[i] == '-' || text[i] == '_') {
			i++
		}
		if !sensitiveKey(text[keyStart:i]) {
			return 0, false
		}
	}
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	if i >= len(text) || (text[i] != ':' && text[i] != '=') {
		return 0, false
	}
	i++
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	return i, i < len(text)
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
	for _, candidate := range []string{"apikey", "password", "token", "secret", "credential", "environment", "buildarg", "buildsecret", "registrypassword", "registrycredential"} {
		if key == candidate {
			return true
		}
	}
	return false
}

func quotedValueEnd(text string, start int, quote byte) int {
	escaped := false
	for i := start; i < len(text); i++ {
		if escaped {
			escaped = false
			continue
		}
		if text[i] == '\\' {
			escaped = true
		} else if text[i] == quote {
			return i
		}
	}
	return len(text)
}

func unquotedValueEnd(text string, start int) int {
	for i := start; i < len(text); i++ {
		if strings.ContainsRune(" \t\r\n,;}", rune(text[i])) {
			return i
		}
	}
	return len(text)
}

func isWord(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')
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
