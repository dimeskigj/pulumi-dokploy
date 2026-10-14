package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func bitbucketLifecycleState() BitbucketIntegrationState {
	return BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "old", APIToken: stringPtr("prior-api"), BitbucketEmail: stringPtr("user@example.invalid"), BitbucketWorkspaceName: stringPtr("old-workspace")}, BitbucketID: "relation", GitProviderID: "parent", OrganizationID: "org"}
}

func bitbucketLifecycleObservedState() BitbucketIntegrationState {
	return BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "old", APIToken: stringPtr("api-secret"), AppPassword: stringPtr("app-secret"), BitbucketEmail: stringPtr("user@example.invalid"), BitbucketUsername: stringPtr("user"), BitbucketWorkspaceName: stringPtr("workspace")}, BitbucketID: "relation", GitProviderID: "parent", OrganizationID: "org"}
}

func bitbucketLifecycleClient(t *testing.T, handler http.Handler) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	api, err := client.New(server.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return api
}

func bitbucketLifecycleRecord(name, apiToken, appPassword, email, username, workspace string) string {
	credential := func(key, value string) string {
		if value == "<omitted>" {
			return ""
		}
		if value == "<null>" {
			return `,"` + key + `":null`
		}
		return `,"` + key + `":"` + value + `"`
	}
	optional := credential
	return `{"bitbucketId":"relation","gitProviderId":"parent"` + credential("apiToken", apiToken) + credential("appPassword", appPassword) + optional("bitbucketEmail", email) + optional("bitbucketUsername", username) + optional("bitbucketWorkspaceName", workspace) + `,"gitProvider":{"gitProviderId":"parent","name":"` + name + `","providerType":"bitbucket","organizationId":"org","userId":"owner"}}`
}

func bitbucketUpdateInputs() BitbucketIntegrationArgs {
	return BitbucketIntegrationArgs{Name: "new", APIToken: stringPtr("new-api"), BitbucketEmail: stringPtr("user@example.invalid"), BitbucketWorkspaceName: stringPtr("")}
}

func TestBitbucketUpdateRequiresFreshCredentialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, active, inactive string
		wantErr                bool
	}{
		{name: "fresh active and explicit null clear", active: "new-api", inactive: "<null>"},
		{name: "fresh active and explicit empty clear", active: "new-api", inactive: ""},
		{name: "omitted active cannot borrow prior", active: "<omitted>", inactive: "<null>", wantErr: true},
		{name: "omitted inactive does not prove clear", active: "new-api", inactive: "<omitted>", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			mutations := 0
			var mutation map[string]any
			api := bitbucketLifecycleClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					reads++
					if reads == 1 {
						fmt.Fprint(w, bitbucketRecord)
					} else {
						fmt.Fprint(w, bitbucketLifecycleRecord("new", tc.active, tc.inactive, "user@example.invalid", "<omitted>", "<null>"))
					}
				case "/api/bitbucket.update":
					mutations++
					body, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(body, &mutation)
					fmt.Fprint(w, `{}`)
				default:
					http.NotFound(w, r)
				}
			}))
			inputs := bitbucketUpdateInputs()
			got, err := (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Update(t.Context(), infer.UpdateRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: bitbucketLifecycleState(), Inputs: inputs})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, "", mutation["appPassword"])
			require.Equal(t, 1, mutations, "update mutation must not be retried")
			require.Equal(t, "new-api", mutation["apiToken"])
			require.Equal(t, "relation", mutation["bitbucketId"])
			require.Equal(t, "parent", mutation["gitProviderId"])
			require.Equal(t, "new", mutation["name"])
			require.Equal(t, "user@example.invalid", mutation["bitbucketEmail"])
			require.Equal(t, "", mutation["bitbucketWorkspaceName"])
			require.NotContains(t, mutation, "bitbucketUsername", "omitted optional username must not be sent")
			for _, forbidden := range []string{"organizationId", "authId", "userId", "owner", "sharing"} {
				require.NotContains(t, mutation, forbidden)
			}
			require.NotContains(t, string(bitbucketJSON(t, mutation)), "null")
			if !tc.wantErr {
				require.Nil(t, got.Output.AppPassword)
				require.Nil(t, got.Output.BitbucketWorkspaceName)
			}
		})
	}
}

func TestBitbucketUpdatePartialState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		malformed bool
		after     string
	}{
		{name: "acknowledged update mismatch returns actual state", status: http.StatusOK, after: bitbucketLifecycleRecord("new", "new-api", "", "user@example.invalid", "<omitted>", "old-workspace")},
		{name: "mutation failure reads actual state", status: http.StatusBadRequest, after: bitbucketLifecycleRecord("server-name", "prior-api", "", "user@example.invalid", "<omitted>", "old-workspace")},
		{name: "malformed readback preserves verified prior", status: http.StatusOK, malformed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			api := bitbucketLifecycleClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					reads++
					if reads == 1 {
						fmt.Fprint(w, bitbucketRecord)
					} else if tc.malformed {
						fmt.Fprint(w, `{"apiToken":"credential-sentinel https://private.invalid"`)
					} else {
						fmt.Fprint(w, tc.after)
					}
				case "/api/bitbucket.update":
					w.WriteHeader(tc.status)
					fmt.Fprint(w, `{"message":"credential-sentinel https://private.invalid"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			prior := bitbucketLifecycleState()
			got, err := (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Update(t.Context(), infer.UpdateRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: prior, Inputs: bitbucketUpdateInputs()})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "credential-sentinel")
			require.NotContains(t, err.Error(), "private.invalid")
			switch {
			case tc.malformed:
				require.Equal(t, bitbucketLifecycleObservedState(), got.Output)
			case tc.status != http.StatusOK:
				require.Equal(t, "server-name", got.Output.Name)
				require.Nil(t, got.Output.APIToken)
			default:
				require.Equal(t, "new", got.Output.Name)
				require.Nil(t, got.Output.AppPassword)
				require.Equal(t, "old-workspace", *got.Output.BitbucketWorkspaceName)
			}
		})
	}
}

func TestBitbucketUpdateRejectsPostMutationIdentityDrift(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drift func(string) string
	}{
		{
			name: "relation ID",
			drift: func(record string) string {
				return strings.Replace(record, `"bitbucketId":"relation"`, `"bitbucketId":"unverified-relation"`, 1)
			},
		},
		{
			name: "parent ID with matching nested parent",
			drift: func(record string) string {
				record = strings.Replace(record, `"gitProviderId":"parent"`, `"gitProviderId":"unverified-parent"`, 1)
				return strings.Replace(record, `"gitProviderId":"parent"`, `"gitProviderId":"unverified-parent"`, 1)
			},
		},
		{
			name: "organization ID",
			drift: func(record string) string {
				return strings.Replace(record, `"organizationId":"org"`, `"organizationId":"unverified-organization"`, 1)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, mutations := 0, 0
			var paths []string
			api := bitbucketLifecycleClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					reads++
					if reads == 1 {
						fmt.Fprint(w, bitbucketRecord)
					} else {
						fmt.Fprint(w, tc.drift(bitbucketRecord))
					}
				case "/api/bitbucket.update":
					mutations++
					fmt.Fprint(w, `{}`)
				default:
					http.NotFound(w, r)
				}
			}))
			got, err := (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Update(t.Context(), infer.UpdateRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: bitbucketLifecycleState(), Inputs: bitbucketUpdateInputs()})
			require.Error(t, err)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.Equal(t, bitbucketLifecycleObservedState(), got.Output, "identity drift must return only the last verified state")
			for _, unverified := range []string{"unverified-relation", "unverified-parent", "unverified-organization"} {
				require.NotContains(t, err.Error(), unverified)
			}
			require.NotContains(t, err.Error(), "api-secret")
			require.NotContains(t, err.Error(), "app-secret")
			require.Equal(t, 1, mutations, "identity mismatch must not retry the mutation")
			require.Equal(t, []string{"/api/organization.active", "/api/bitbucket.one", "/api/bitbucket.update", "/api/bitbucket.one"}, paths)
		})
	}
}

func TestBitbucketDeleteNotFoundClassificationAndSafeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		preStatus, removeStatus, postStatus int
		wantSuccess                         bool
		wantRemoves                         int
	}{
		{name: "pre-read not found does not remove", preStatus: http.StatusNotFound, wantSuccess: true},
		{name: "pre-read bad request is not absence", preStatus: http.StatusBadRequest},
		{name: "remove bad request and still present is not absence", removeStatus: http.StatusBadRequest, postStatus: http.StatusOK, wantRemoves: 1},
		{name: "remove error followed by verified absence succeeds", removeStatus: http.StatusBadRequest, postStatus: http.StatusNotFound, wantSuccess: true, wantRemoves: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, removes := 0, 0
			api := bitbucketLifecycleClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					reads++
					status := http.StatusOK
					if reads == 1 && tc.preStatus != 0 {
						status = tc.preStatus
					}
					if reads > 1 && tc.postStatus != 0 {
						status = tc.postStatus
					}
					w.WriteHeader(status)
					if status == http.StatusOK {
						fmt.Fprint(w, bitbucketRecord)
					} else {
						fmt.Fprint(w, `{"message":"credential-sentinel https://private.invalid"}`)
					}
				case "/api/gitProvider.remove":
					removes++
					var body map[string]string
					raw, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(raw, &body)
					require.Equal(t, map[string]string{"gitProviderId": "parent"}, body)
					w.WriteHeader(tc.removeStatus)
					fmt.Fprint(w, `{"message":"BAD_REQUEST credential-sentinel"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			_, err := (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Delete(t.Context(), infer.DeleteRequest[BitbucketIntegrationState]{ID: "relation", State: bitbucketLifecycleState()})
			if tc.wantSuccess {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "credential-sentinel")
				require.NotContains(t, err.Error(), "private.invalid")
				require.NotContains(t, err.Error(), "parent")
			}
			require.Equal(t, tc.wantRemoves, removes)
		})
	}
}

func TestBitbucketUpdateCancellation(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					fmt.Fprint(w, bitbucketRecord)
				case "/api/bitbucket.update":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/bitbucket.update" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			prior := bitbucketLifecycleState()
			got, err := (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Update(ctx, infer.UpdateRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{ID: "relation", State: prior, Inputs: bitbucketUpdateInputs()})
			require.ErrorIs(t, err, cause)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.Equal(t, bitbucketLifecycleObservedState(), got.Output)
			require.Equal(t, []string{"/api/organization.active", "/api/bitbucket.one", "/api/bitbucket.update"}, paths)
		})
	}
}

func TestBitbucketDeleteCancellation(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					fmt.Fprint(w, bitbucketRecord)
				case "/api/gitProvider.remove":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/gitProvider.remove" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			_, err = (BitbucketIntegration{client: func(context.Context) *client.Client { return api }}).Delete(ctx, infer.DeleteRequest[BitbucketIntegrationState]{ID: "relation", State: bitbucketLifecycleState()})
			require.ErrorIs(t, err, cause)
			require.Equal(t, []string{"/api/organization.active", "/api/bitbucket.one", "/api/gitProvider.remove"}, paths)
		})
	}
}

func TestBitbucketMutationClosesBody(t *testing.T) {
	body := &lifecycleTrackingBody{}
	resp := &http.Response{StatusCode: http.StatusBadRequest, Body: body}
	err := bitbucketMutation(resp, nil, "bitbucket.update")
	require.Error(t, err)
	require.True(t, body.closed)
	require.NotContains(t, err.Error(), "private")
}

type lifecycleTrackingBody struct{ closed bool }

func (b *lifecycleTrackingBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *lifecycleTrackingBody) Close() error             { b.closed = true; return nil }

func bitbucketJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
