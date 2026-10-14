package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

const bitbucketRecord = `{"bitbucketId":"relation","gitProviderId":"parent","apiToken":"api-secret","appPassword":"app-secret","bitbucketEmail":"user@example.invalid","bitbucketUsername":"user","bitbucketWorkspaceName":"workspace","gitProvider":{"gitProviderId":"parent","name":"old","providerType":"bitbucket","organizationId":"org","userId":"owner"}}`

func bitbucketFixture(t *testing.T, record string, status int) *client.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/organization.active" {
			fmt.Fprint(w, `{"id":"org"}`)
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, record)
	}))
	t.Cleanup(s.Close)
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return c
}

func TestReadBitbucketIntegration(t *testing.T) {
	t.Run("imports valid, incomplete, and dual-credential records", func(t *testing.T) {
		for name, record := range map[string]string{
			"api token":        bitbucketRecord,
			"app password":     strings.Replace(strings.Replace(bitbucketRecord, `"apiToken":"api-secret"`, `"apiToken":""`, 1), `"bitbucketEmail":"user@example.invalid",`, "", 1),
			"incomplete":       strings.Replace(strings.Replace(bitbucketRecord, `"apiToken":"api-secret",`, "", 1), `"appPassword":"app-secret",`, "", 1),
			"both credentials": bitbucketRecord,
		} {
			t.Run(name, func(t *testing.T) {
				resource := BitbucketIntegration{client: func(context.Context) *client.Client { return bitbucketFixture(t, record, http.StatusOK) }}
				got, err := resource.Read(t.Context(), infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation"})
				require.NoError(t, err)
				require.Equal(t, "relation", got.ID)
				require.Equal(t, "parent", got.State.GitProviderID)
				require.Equal(t, "org", got.State.OrganizationID)
				require.Equal(t, "old", got.Inputs.Name)
				observed, err := readBitbucketIntegration(t.Context(), resource.client(t.Context()), "relation", BitbucketIntegrationState{}, "org")
				require.NoError(t, err)
				switch name {
				case "incomplete":
					require.False(t, observed.APITokenEvidence.Specified)
					require.False(t, observed.AppPasswordEvidence.Specified)
				case "app password":
					require.True(t, observed.APITokenEvidence.Cleared)
					require.Equal(t, "app-secret", observed.AppPasswordEvidence.Value)
				default:
					require.True(t, observed.APITokenEvidence.Specified)
					require.Equal(t, "api-secret", observed.APITokenEvidence.Value)
				}
			})
		}
	})

	t.Run("rejects mismatched relation parent type active and prior identities", func(t *testing.T) {
		cases := []struct {
			name   string
			record string
			prior  BitbucketIntegrationState
		}{
			{name: "relation", record: strings.Replace(bitbucketRecord, `"bitbucketId":"relation"`, `"bitbucketId":"other"`, 1)},
			{name: "parent", record: strings.Replace(bitbucketRecord, `"gitProviderId":"parent","apiToken"`, `"gitProviderId":"other","apiToken"`, 1)},
			{name: "nested parent", record: strings.Replace(bitbucketRecord, `"gitProviderId":"parent","name":"old"`, `"gitProviderId":"other","name":"old"`, 1)},
			{name: "type", record: strings.Replace(bitbucketRecord, `"providerType":"bitbucket"`, `"providerType":"gitlab"`, 1)},
			{name: "active organization", record: strings.Replace(bitbucketRecord, `"organizationId":"org"`, `"organizationId":"other"`, 1)},
			{name: "missing owner", record: strings.Replace(bitbucketRecord, `,"userId":"owner"`, "", 1)},
			{name: "prior relation", record: bitbucketRecord, prior: BitbucketIntegrationState{BitbucketID: "other"}},
			{name: "prior parent", record: bitbucketRecord, prior: BitbucketIntegrationState{GitProviderID: "other"}},
			{name: "prior organization", record: bitbucketRecord, prior: BitbucketIntegrationState{OrganizationID: "other"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				resource := BitbucketIntegration{client: func(context.Context) *client.Client { return bitbucketFixture(t, tc.record, http.StatusOK) }}
				_, err := resource.Read(t.Context(), infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: tc.prior})
				require.Error(t, err)
			})
		}
	})

	t.Run("normalizes credential null empty and omission while preserving prior secrets", func(t *testing.T) {
		for _, tc := range []struct {
			name, fragment, want string
			specified, cleared   bool
		}{
			{name: "null", fragment: `"apiToken":null,`, want: "previous", specified: true, cleared: true},
			{name: "omitted", fragment: "", want: "previous"},
			{name: "empty", fragment: `"apiToken":"",`, want: "", specified: true, cleared: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				record := strings.Replace(bitbucketRecord, `"apiToken":"api-secret",`, tc.fragment, 1)
				resource := BitbucketIntegration{client: func(context.Context) *client.Client { return bitbucketFixture(t, record, http.StatusOK) }}
				prior := BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{APIToken: stringPtr("previous")}}
				got, err := resource.Read(t.Context(), infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: prior})
				require.NoError(t, err)
				observed, err := readBitbucketIntegration(t.Context(), resource.client(t.Context()), "relation", prior, "org")
				require.NoError(t, err)
				require.Equal(t, tc.specified, observed.APITokenEvidence.Specified)
				require.Equal(t, tc.cleared, observed.APITokenEvidence.Cleared)
				if tc.want == "" {
					require.Nil(t, got.State.APIToken)
				} else {
					require.Equal(t, tc.want, *got.State.APIToken)
				}
			})
		}
	})

	t.Run("sanitizes malformed and error bodies", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			body   string
		}{{http.StatusOK, `{"secret":"api-secret","url":"https://private.invalid"`}, {http.StatusBadRequest, `{"message":"api-secret https://private.invalid"}`}} {
			resource := BitbucketIntegration{client: func(context.Context) *client.Client { return bitbucketFixture(t, tc.body, tc.status) }}
			_, err := resource.Read(t.Context(), infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation"})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "api-secret")
			require.NotContains(t, err.Error(), "private.invalid")
		}
	})

	t.Run("only a genuine not-found clears imported state", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			clears bool
		}{{http.StatusNotFound, true}, {http.StatusBadRequest, false}} {
			resource := BitbucketIntegration{client: func(context.Context) *client.Client {
				return bitbucketFixture(t, `{"message":"credential-sentinel"}`, tc.status)
			}}
			got, err := resource.Read(t.Context(), infer.ReadRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation"})
			if tc.clears {
				require.NoError(t, err)
				require.Empty(t, got.ID)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "credential-sentinel")
				require.Empty(t, got.ID)
			}
		}
	})
}

func TestBitbucketCredentialEvidenceAndSettings(t *testing.T) {
	desired := BitbucketIntegrationArgs{Name: "new", APIToken: stringPtr("rotated"), BitbucketEmail: stringPtr("user@example.invalid")}
	good := bitbucketObservation{State: BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "new", APIToken: stringPtr("rotated"), BitbucketEmail: stringPtr("user@example.invalid")}, BitbucketID: "relation", GitProviderID: "parent", OrganizationID: "org"}, OwnerUserID: "owner", APITokenEvidence: bitbucketCredentialEvidence{Specified: true, Value: "rotated"}, AppPasswordEvidence: bitbucketCredentialEvidence{Specified: true, Cleared: true}}
	require.True(t, bitbucketIntegrationSettingsMatch(good, desired, "new"))
	stale := good
	stale.State.APIToken = stringPtr("old-secret")
	stale.APITokenEvidence = bitbucketCredentialEvidence{Specified: false}
	require.False(t, bitbucketIntegrationSettingsMatch(stale, desired, "new"), "preserved prior token cannot confirm a rotation")
	uncleared := good
	uncleared.AppPasswordEvidence = bitbucketCredentialEvidence{Specified: false}
	require.False(t, bitbucketIntegrationSettingsMatch(uncleared, desired, "new"), "omission cannot confirm inactive credential clear")
	wrongName := good
	wrongName.State.Name = "other"
	require.False(t, bitbucketIntegrationSettingsMatch(wrongName, desired, "new"))
	wrongIdentity := good
	wrongIdentity.State.BitbucketEmail = stringPtr("other@example.invalid")
	require.False(t, bitbucketIntegrationSettingsMatch(wrongIdentity, desired, "new"))
	optionalUnmanaged := desired
	optionalUnmanaged.BitbucketWorkspaceName = nil
	good.State.BitbucketWorkspaceName = stringPtr("unmanaged-existing")
	require.True(t, bitbucketIntegrationSettingsMatch(good, optionalUnmanaged, "new"), "omitted optional fields are observational only")
	optionalClear := optionalUnmanaged
	optionalClear.BitbucketWorkspaceName = stringPtr("")
	clearedWorkspace := good
	clearedWorkspace.State.BitbucketWorkspaceName = nil
	require.True(t, bitbucketIntegrationSettingsMatch(clearedWorkspace, optionalClear, "new"), "explicit empty workspace confirms a cleared value")
	require.False(t, bitbucketIntegrationSettingsMatch(good, optionalClear, "new"), "explicit empty workspace must match a cleared value")

	appDesired := BitbucketIntegrationArgs{Name: "new", AppPassword: stringPtr("legacy-secret"), BitbucketUsername: stringPtr("user")}
	appObserved := bitbucketObservation{State: BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "new", AppPassword: stringPtr("legacy-secret"), BitbucketUsername: stringPtr("user")}, BitbucketID: "relation", GitProviderID: "parent", OrganizationID: "org"}, OwnerUserID: "owner", APITokenEvidence: bitbucketCredentialEvidence{Specified: true, Cleared: true}, AppPasswordEvidence: bitbucketCredentialEvidence{Specified: true, Value: "legacy-secret"}}
	require.True(t, bitbucketIntegrationSettingsMatch(appObserved, appDesired, "new"))

	priorSecret := BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{AppPassword: stringPtr("old-secret")}}
	clearDesired := BitbucketIntegrationArgs{APIToken: stringPtr("rotated")}
	priorSecret.APIToken = stringPtr("old-api-secret")
	cleared := bitbucketObservation{State: priorSecret, APITokenEvidence: bitbucketCredentialEvidence{Specified: false}, AppPasswordEvidence: bitbucketCredentialEvidence{Specified: true, Cleared: true}}
	projected := bitbucketIntegrationMutationState(cleared, clearDesired)
	require.Nil(t, projected.AppPassword, "an explicitly cleared inactive secret must not survive refresh preservation")
	require.Nil(t, projected.APIToken, "an unconfirmed desired rotation must not be copied from desired settings")
	require.Equal(t, "old-secret", *priorSecret.AppPassword)
}

func TestBitbucketIntegrationErrorSanitization(t *testing.T) {
	unsafe := errors.New("credential-sentinel https://private.invalid")
	got := sanitizeBitbucketIntegrationError(unsafe)
	require.NotContains(t, got.Error(), "credential-sentinel")
	require.NotContains(t, got.Error(), "private.invalid")
	for _, ctxErr := range []error{context.Canceled, context.DeadlineExceeded} {
		got := sanitizeBitbucketIntegrationError(errors.Join(unsafe, ctxErr))
		require.ErrorIs(t, got, ctxErr)
		require.NotContains(t, got.Error(), "credential-sentinel")
		require.NotContains(t, got.Error(), "private.invalid")
	}
}
