package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
)

func validateLookupID(field, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

func validateLookupIdentity(operation, requestedID string, returnedID, name *string) error {
	if returnedID == nil || *returnedID == "" || *returnedID != requestedID || name == nil {
		return lookupResponseError(operation)
	}
	return nil
}

func lookupResponseError(operation string) error {
	return fmt.Errorf("%s: invalid response contract", operation)
}

func lookupOptionalString(operation string, fields map[string]any, field string) (*string, error) {
	value, ok := fields[field]
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, lookupResponseError(operation)
	}
	return &text, nil
}

func lookupError(operation string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: request canceled: %w", operation, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: request deadline exceeded: %w", operation, context.DeadlineExceeded)
	}
	if client.IsNotFound(err) {
		return fmt.Errorf("%s: not found", operation)
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return fmt.Errorf("%s: authentication failed", operation)
		case 403:
			return fmt.Errorf("%s: authorization failed", operation)
		default:
			return fmt.Errorf("%s: API request failed (HTTP %d)", operation, apiErr.StatusCode)
		}
	}
	var syntax *json.SyntaxError
	var typ *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typ) {
		return lookupResponseError(operation)
	}
	return fmt.Errorf("%s: request failed", operation)
}

func safeRegistryURL(raw *string) *string {
	if raw == nil || *raw == "" {
		return raw
	}
	if strings.ContainsAny(*raw, "?#") {
		return nil
	}
	parse := *raw
	if !strings.Contains(parse, "://") {
		parse = "//" + parse
	}
	u, err := url.Parse(parse)
	if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil
	}
	return raw
}
