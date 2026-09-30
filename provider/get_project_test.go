package dokploy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestLookupControlPlaneProject(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		status      int
		fail        bool
	}{
		{"complete", `{"projectId":"id","name":"display","description":"","defaultEnvironmentId":null,"futureSecret":"mock-secret"}`, 200, false},
		{"default environment", `{"projectId":"id","name":"display","defaultEnvironmentId":"environment-id"}`, 200, false},
		{"minimal", `{"projectId":"id","name":""}`, 200, false},
		{"null object", `null`, 200, true},
		{"missing id", `{"name":"x"}`, 200, true},
		{"null name", `{"projectId":"id","name":null}`, 200, true},
		{"wrong optional type", `{"projectId":"id","name":"x","description":{"secret":"mock-secret"}}`, 200, true},
		{"missing name", `{"projectId":"id"}`, 200, true},
		{"mismatched id", `{"projectId":"other","name":"x"}`, 200, true},
		{"wrong type", `{"projectId":"id","name":123}`, 200, true},
		{"invalid json", `{`, 200, true},
		{"not found", `{"message":"mock-secret"}`, 404, true},
		{"forbidden", `{"message":"mock-secret"}`, 403, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: url.Values{"projectId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			r := GetProject{client: fixedClient(s.API())}
			out, err := r.Invoke(t.Context(), infer.FunctionRequest[GetProjectArgs]{Input: GetProjectArgs{ProjectID: "id"}})
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				if tc.title == "invalid json" || tc.title == "wrong type" || tc.title == "wrong optional type" || tc.title == "missing name" || tc.title == "mismatched id" || tc.title == "null object" || tc.title == "missing id" || tc.title == "null name" {
					require.Contains(t, err.Error(), "response contract")
				}
				if tc.title == "not found" {
					require.Contains(t, err.Error(), "not found")
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, "id", out.Output.ProjectID)
			if tc.title == "minimal" {
				require.Equal(t, "", out.Output.Name)
				require.Nil(t, out.Output.Description)
				require.Nil(t, out.Output.DefaultEnvironmentID)
			} else {
				require.Equal(t, "display", out.Output.Name)
			}
			if tc.title == "complete" {
				require.Equal(t, "", *out.Output.Description)
				require.Nil(t, out.Output.DefaultEnvironmentID)
			}
			if tc.title == "default environment" {
				require.Equal(t, "environment-id", *out.Output.DefaultEnvironmentID)
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		r := GetProject{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := r.Invoke(t.Context(), infer.FunctionRequest[GetProjectArgs]{Input: GetProjectArgs{ProjectID: id}})
		require.Error(t, err)
		require.False(t, called)
	}
}

func TestLookupOpaqueIDRoundTrip(t *testing.T) {
	id := " opaque+/percent% "
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: url.Values{"projectId": {id}}, Status: 200, Response: mustJSON(map[string]any{"projectId": id, "name": ""})})
	out, err := (GetProject{client: fixedClient(s.API())}).Invoke(t.Context(), infer.FunctionRequest[GetProjectArgs]{Input: GetProjectArgs{ProjectID: id}})
	require.NoError(t, err)
	require.Equal(t, id, out.Output.ProjectID)
}

func TestLookupRequestCancellation(t *testing.T) {
	for _, category := range []struct {
		name  string
		cause error
	}{{"canceled", context.Canceled}, {"deadline", context.DeadlineExceeded}} {
		t.Run(category.name, func(t *testing.T) {
			received := make(chan struct{})
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: url.Values{"projectId": {"id"}}, Received: received, WaitForContextDone: true})
			ctx, cancel := context.WithCancel(t.Context())
			if category.cause == context.DeadlineExceeded {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithDeadline(t.Context(), time.Now().Add(time.Second))
				defer deadlineCancel()
			}
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := (GetProject{client: fixedClient(s.API())}).Invoke(ctx, infer.FunctionRequest[GetProjectArgs]{Input: GetProjectArgs{ProjectID: "id"}})
				result <- err
			}()
			select {
			case <-received:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not arrive")
			}
			if category.cause == context.Canceled {
				cancel()
			}
			select {
			case err := <-result:
				require.Error(t, err)
				require.True(t, errors.Is(err, category.cause), "expected context category")
				require.NotContains(t, err.Error(), s.server.URL)
			case <-time.After(3 * time.Second):
				t.Fatal("request did not terminate promptly")
			}
		})
	}
}
