package dokploy

import (
	"context"
	"net/http"
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// Dokploy v0.30.8 wraps both missing-port and permission errors as this 400.
const portAmbiguousMissing = `{"code":"BAD_REQUEST","message":"Port not found"}`

func portErrorStep(t *testing.T, method, route string, status int, body string) func(http.ResponseWriter, *http.Request) {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, method, req.Method)
		require.Equal(t, "/api/port."+route, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestPortAmbiguousAbsenceRequiresOwnerEvidence(t *testing.T) {
	for _, operation := range []string{"read", "delete"} {
		for _, tc := range []struct {
			name, owner, relation string
			status                int
			wantAbsent            bool
		}{
			{"empty", "a1", `{"applicationId":"a1","ports":[]}`, 200, true},
			{"other-port", "a1", `{"applicationId":"a1","ports":[{"portId":"p2"}]}`, 200, true},
			{"still-present", "a1", `{"applicationId":"a1","ports":[{"portId":"p1"}]}`, 200, false},
			{"unknown-owner", "", "", 0, false},
			{"wrong-owner", "a1", `{"applicationId":"a2","ports":[]}`, 200, false},
			{"missing-relation", "a1", `{"applicationId":"a1"}`, 200, false},
			{"null-relation", "a1", `{"applicationId":"a1","ports":null}`, 200, false},
			{"invalid-entry", "a1", `{"applicationId":"a1","ports":[{}]}`, 200, false},
			{"null-entry", "a1", `{"applicationId":"a1","ports":[null]}`, 200, false},
			{"forbidden", "a1", `{"code":"FORBIDDEN"}`, 403, false},
			{"missing-parent", "a1", `{"code":"NOT_FOUND"}`, 404, false},
			{"malformed", "a1", `{`, 200, false},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				method, route := http.MethodGet, "one"
				if operation == "delete" {
					method, route = http.MethodPost, "delete"
				}
				steps := []func(http.ResponseWriter, *http.Request){portErrorStep(t, method, route, 400, portAmbiguousMissing)}
				if tc.owner != "" {
					steps = append(steps, func(w http.ResponseWriter, req *http.Request) {
						require.Equal(t, http.MethodGet, req.Method)
						require.Equal(t, "/api/application.one", req.URL.Path)
						require.Equal(t, "a1", req.URL.Query().Get("applicationId"))
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tc.status)
						_, _ = w.Write([]byte(tc.relation))
					})
				}
				r, calls := portFixture(t, steps...)
				state := PortState{PortArgs: PortArgs{ApplicationID: tc.owner}, PortID: "p1"}
				var err error
				if operation == "read" {
					var response infer.ReadResponse[PortArgs, PortState]
					response, err = r.Read(context.Background(), infer.ReadRequest[PortArgs, PortState]{ID: "p1", State: state})
					require.Empty(t, response.ID)
				} else {
					_, err = r.Delete(context.Background(), infer.DeleteRequest[PortState]{ID: "p1", State: state})
				}
				if tc.wantAbsent {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.NotContains(t, err.Error(), "Port not found", "withhold upstream prose")
				}
				require.Equal(t, len(steps), *calls)
			})
		}
	}
}

func TestPortOtherFailuresDoNotProbeOwner(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{400, `{"code":"BAD_REQUEST","message":"Invalid input"}`},
		{403, `{"code":"FORBIDDEN","message":"Port not found"}`},
		{500, `{"code":"INTERNAL_SERVER_ERROR","message":"Port not found"}`},
	} {
		r, calls := portFixture(t, portErrorStep(t, http.MethodGet, "one", tc.status, tc.body))
		_, err := r.Read(context.Background(), infer.ReadRequest[PortArgs, PortState]{ID: "p1", State: PortState{PortArgs: portArgs()}})
		require.Error(t, err)
		require.Equal(t, 1, *calls)
	}
}
