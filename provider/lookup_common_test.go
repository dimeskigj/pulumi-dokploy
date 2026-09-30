package dokploy

import (
	"context"
	"errors"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

func TestLookupValidationAndErrors(t *testing.T) {
	require.Error(t, validateLookupID("projectId", " \t\n"))
	require.NoError(t, validateLookupID("projectId", " opaque+/percent% "))
	require.Error(t, validateLookupIdentity("project.one", "id", nil, stringPtr("")))
	require.Error(t, validateLookupIdentity("project.one", "id", stringPtr(""), stringPtr("")))
	require.Error(t, validateLookupIdentity("project.one", "id", stringPtr("other"), stringPtr("")))
	require.Error(t, validateLookupIdentity("project.one", "id", stringPtr("id"), nil))
	require.NoError(t, validateLookupIdentity("project.one", "id", stringPtr("id"), stringPtr("")))
	for _, tc := range []struct {
		err      error
		category string
	}{
		{&client.APIError{StatusCode: 404, Message: "mock-id private-key mock-token"}, "not found"},
		{&client.APIError{StatusCode: 400, Code: "NOT_FOUND", Message: "mock-id"}, "not found"},
		{&client.APIError{StatusCode: 401, Message: "mock-token"}, "authentication"},
		{&client.APIError{StatusCode: 403, Message: "mock-token"}, "authorization"},
		{errors.New("https://private.example/mock-id private-key mock-token"), "request failed"},
		{context.Canceled, "canceled"}, {context.DeadlineExceeded, "deadline"},
	} {
		err := lookupError("project.one", tc.err)
		require.Contains(t, err.Error(), "project.one")
		require.Contains(t, err.Error(), tc.category)
		for _, secret := range []string{"mock-id", "private-key", "mock-token", "private.example"} {
			require.NotContains(t, err.Error(), secret)
		}
	}
	require.ErrorIs(t, lookupError("project.one", context.Canceled), context.Canceled)
	require.ErrorIs(t, lookupError("project.one", context.DeadlineExceeded), context.DeadlineExceeded)
	require.Contains(t, lookupResponseError("project.one").Error(), "response contract")
}

func TestLookupRegistryURLSafety(t *testing.T) {
	require.Nil(t, safeRegistryURL(nil))
	for _, raw := range []string{"https://user:mock-password@registry.example", "registry.example:5000?token=mock-token", "https://registry.example/#secret", "%invalid"} {
		require.Nil(t, safeRegistryURL(&raw))
	}
	for _, raw := range []string{"", "registry.example:5000", "https://registry.example:5000/path"} {
		require.Equal(t, raw, *safeRegistryURL(&raw))
	}
}
