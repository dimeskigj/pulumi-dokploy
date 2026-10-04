package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

const portRow = `{"portId":"p1","applicationId":"a1","publishedPort":8081,"targetPort":80,"protocol":"tcp","publishMode":"ingress"}`

func portArgs() PortArgs {
	return PortArgs{ApplicationID: "a1", PublishedPort: 8080, TargetPort: 81, Protocol: ptr(PortProtocolUDP), PublishMode: ptr(PortPublishModeHost)}
}

func portFixture(t *testing.T, steps ...func(http.ResponseWriter, *http.Request)) (Port, *int) {
	t.Helper()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls >= len(steps) {
			t.Error("unexpected request")
			w.WriteHeader(500)
			return
		}
		step := steps[calls]
		calls++
		step(w, req)
	}))
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "placeholder", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return Port{client: fixedClient(api)}, &calls
}

func portStep(t *testing.T, method, path, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, method, req.Method)
		require.Equal(t, "/api/port."+path, req.URL.Path)
		if path == "one" {
			require.Equal(t, "p1", req.URL.Query().Get("portId"))
		}
		if path == "update" || path == "delete" {
			var b map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
			require.Equal(t, "p1", b["portId"])
			if path == "update" {
				require.Len(t, b, 5)
				for k, v := range map[string]any{"publishedPort": float64(8080), "protocol": "udp", "publishMode": "host"} {
					require.Equal(t, v, b[k], k)
				}
				require.Contains(t, []any{float64(81), float64(82)}, b["targetPort"])
			}
			if path == "delete" {
				require.Len(t, b, 1)
			}
		}
		if path == "create" {
			var b map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
			require.Len(t, b, 5)
			for k, v := range map[string]any{"applicationId": "a1", "publishedPort": float64(8080), "targetPort": float64(81), "protocol": "udp", "publishMode": "host"} {
				require.Equal(t, v, b[k], k)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func TestPortCreateAndReadBack(t *testing.T) {
	r, calls := portFixture(t, portStep(t, "POST", "create", `{"portId":"p1"}`), portStep(t, "GET", "one", portRow))
	got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, PortState{PortArgs: PortArgs{ApplicationID: "a1", PublishedPort: 8081, TargetPort: 80, Protocol: ptr(PortProtocolTCP), PublishMode: ptr(PortPublishModeIngress)}, PortID: "p1"}, got.Output)
	require.Equal(t, 2, *calls)
}

func TestPortImportAndRefresh(t *testing.T) {
	r, calls := portFixture(t, portStep(t, "GET", "one", portRow))
	got, err := r.Read(t.Context(), infer.ReadRequest[PortArgs, PortState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, 8081, got.Inputs.PublishedPort)
	require.Equal(t, got.Inputs, got.State.PortArgs)
	require.Equal(t, 1, *calls)
}

func TestPortUpdateAndReadBack(t *testing.T) {
	r, calls := portFixture(t, portStep(t, "POST", "update", `{invalid`), portStep(t, "GET", "one", portRow))
	got, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: portArgs(), State: PortState{PortArgs: portArgs(), PortID: "p1"}})
	require.NoError(t, err)
	require.Equal(t, 8081, got.Output.PublishedPort)
	require.Equal(t, "p1", got.Output.PortID)
	require.Equal(t, 2, *calls)
}

func TestPortDelete(t *testing.T) {
	r, calls := portFixture(t, portStep(t, "POST", "delete", `{invalid`))
	_, err := r.Delete(t.Context(), infer.DeleteRequest[PortState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, 1, *calls)
}

func TestPortReadRejectsInvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"missing": `{"portId":"p1","applicationId":"a1"}`, "null": `{"portId":"p1","applicationId":"a1","publishedPort":null,"targetPort":80,"protocol":"tcp","publishMode":"ingress"}`,
		"missingOwner":    strings.Replace(portRow, `"applicationId":"a1",`, "", 1),
		"nullOwner":       strings.Replace(portRow, `"applicationId":"a1"`, `"applicationId":null`, 1),
		"missingTarget":   strings.Replace(portRow, `,"targetPort":80`, "", 1),
		"nullTarget":      strings.Replace(portRow, `"targetPort":80`, `"targetPort":null`, 1),
		"missingProtocol": strings.Replace(portRow, `,"protocol":"tcp"`, "", 1),
		"nullProtocol":    strings.Replace(portRow, `"protocol":"tcp"`, `"protocol":null`, 1),
		"missingMode":     strings.Replace(portRow, `,"publishMode":"ingress"`, "", 1),
		"nullMode":        strings.Replace(portRow, `"publishMode":"ingress"`, `"publishMode":null`, 1),
		"owner":           strings.Replace(portRow, `"a1"`, `""`, 1), "zero": strings.Replace(portRow, "8081", "0", 1),
		"large": strings.Replace(portRow, "8081", "65536", 1), "fraction": strings.Replace(portRow, "8081", "80.5", 1),
		"targetZero":     strings.Replace(portRow, `"targetPort":80`, `"targetPort":0`, 1),
		"targetLarge":    strings.Replace(portRow, `"targetPort":80`, `"targetPort":65536`, 1),
		"targetFraction": strings.Replace(portRow, `"targetPort":80`, `"targetPort":80.5`, 1),
		"missingID":      strings.Replace(portRow, `"portId":"p1",`, "", 1),
		"nullID":         strings.Replace(portRow, `"portId":"p1"`, `"portId":null`, 1),
		"protocol":       strings.Replace(portRow, `"tcp"`, `"icmp"`, 1), "mode": strings.Replace(portRow, `"ingress"`, `"other"`, 1),
		"id": strings.Replace(portRow, `"p1"`, `"p2"`, 1), "malformed": "{",
	} {
		t.Run(name, func(t *testing.T) {
			r, calls := portFixture(t, portStep(t, "GET", "one", body))
			got, err := r.Read(t.Context(), infer.ReadRequest[PortArgs, PortState]{ID: "p1"})
			require.Error(t, err)
			require.Empty(t, got.ID)
			require.Equal(t, 1, *calls)
		})
	}
}

func TestPortMutationReadBackRejectsWrongOwner(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, observed := range []string{strings.Replace(portRow, `"a1"`, `"a2"`, 1), strings.Replace(portRow, `"p1"`, `"p2"`, 1)} {
			r, calls := portFixture(t, portStep(t, "POST", map[bool]string{true: "create", false: "update"}[create], `{"portId":"p1"}`), portStep(t, "GET", "one", observed))
			if create {
				got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
				require.Error(t, err)
				require.Equal(t, "p1", got.ID)
				require.Equal(t, portArgs(), got.Output.PortArgs)
			} else {
				got, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: portArgs(), State: PortState{PortArgs: portArgs(), PortID: "p1"}})
				require.Error(t, err)
				require.Equal(t, portArgs(), got.Output.PortArgs)
			}
			require.Equal(t, 2, *calls)
		}
	}
}

func TestPortUpdateRejectsRetargeting(t *testing.T) {
	r := Port{client: func(context.Context) *client.Client { panic("client constructed") }}
	prior := PortState{PortArgs: portArgs(), PortID: "p1"}
	changed := portArgs()
	changed.ApplicationID = "a2"
	got, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: changed, State: prior})
	require.Error(t, err)
	require.Equal(t, prior, got.Output)
}

func TestPortConcreteInvalidInputs(t *testing.T) {
	r := Port{client: func(context.Context) *client.Client { panic("client constructed") }}
	for _, invalid := range []PortArgs{{}, {ApplicationID: "a1", PublishedPort: 0, TargetPort: 80, Protocol: ptr(PortProtocolTCP), PublishMode: ptr(PortPublishModeIngress)}, {ApplicationID: "a1", PublishedPort: 80, TargetPort: 65536, Protocol: ptr(PortProtocolTCP), PublishMode: ptr(PortPublishModeIngress)}} {
		_, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: invalid})
		require.Error(t, err)
		_, err = r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: invalid, State: PortState{PortArgs: portArgs()}})
		require.Error(t, err)
	}
}

func TestPortCreateRetainsAcknowledgedID(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		body := ""
		if confirmed {
			body = portRow
		}
		r, calls := portFixture(t, portStep(t, "POST", "create", `{"portId":"p1","publishedPort":"invalid"}`), portStep(t, "GET", "one", body))
		got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
		if confirmed {
			require.NoError(t, err)
			require.Equal(t, 8081, got.Output.PublishedPort)
		} else {
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.Equal(t, portArgs(), got.Output.PortArgs)
		}
		require.Equal(t, "p1", got.ID)
		require.Equal(t, "p1", got.Output.PortID)
		require.Equal(t, 2, *calls)
	}
	for _, body := range []string{`{}`, `{"portId":null}`, `{"portId":""}`, `{`, `{"portId":"p1","padding":"` + strings.Repeat("x", 1<<20) + `"}`} {
		r, calls := portFixture(t, portStep(t, "POST", "create", body))
		got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
		require.Error(t, err)
		require.Empty(t, got.ID)
		require.Equal(t, 1, *calls)
	}
}

func TestPortUpdateFailureState(t *testing.T) {
	prior := PortState{PortArgs: portArgs(), PortID: "p1"}
	attempted := portArgs()
	attempted.TargetPort = 82
	for _, acknowledged := range []bool{false, true} {
		step := portStep(t, "POST", "update", "")
		if !acknowledged {
			step = func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(403) }
		}
		steps := []func(http.ResponseWriter, *http.Request){step}
		if acknowledged {
			steps = append(steps, portStep(t, "GET", "one", "{}"))
		}
		r, calls := portFixture(t, steps...)
		got, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: attempted, State: prior})
		require.Error(t, err)
		if acknowledged {
			require.Equal(t, attempted, got.Output.PortArgs)
		} else {
			require.Equal(t, prior, got.Output)
		}
		require.Equal(t, len(steps), *calls)
	}
}

func TestPortMutationAcknowledgmentIgnoresUnusedBody(t *testing.T) {
	t.Run("empty update response", func(t *testing.T) {
		r, calls := portFixture(t, portStep(t, "POST", "update", ""), portStep(t, "GET", "one", portRow))
		got, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: portArgs(), State: PortState{PortArgs: portArgs(), PortID: "p1"}})
		require.NoError(t, err)
		require.Equal(t, "p1", got.Output.PortID)
		require.Equal(t, 8081, got.Output.PublishedPort)
		require.Equal(t, 2, *calls)
	})
	t.Run("empty delete response", func(t *testing.T) {
		r, calls := portFixture(t, portStep(t, "POST", "delete", ""))
		_, err := r.Delete(t.Context(), infer.DeleteRequest[PortState]{ID: "p1"})
		require.NoError(t, err)
		require.Equal(t, 1, *calls)
	})
}

func TestPortAbsenceAndErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		absent bool
	}{{404, "", true}, {400, `{"code":"NOT_FOUND"}`, true}, {400, `{"code":"BAD_REQUEST","message":"Port not found"}`, false}, {401, "", false}, {403, "", false}} {
		for _, deletion := range []bool{false, true} {
			path, method := "one", "GET"
			if deletion {
				path, method = "delete", "POST"
			}
			r, calls := portFixture(t, func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, method, req.Method)
				require.Equal(t, "/api/port."+path, req.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			var err error
			if deletion {
				_, err = r.Delete(t.Context(), infer.DeleteRequest[PortState]{ID: "p1"})
			} else {
				var got infer.ReadResponse[PortArgs, PortState]
				got, err = r.Read(t.Context(), infer.ReadRequest[PortArgs, PortState]{ID: "p1"})
				require.Empty(t, got.ID)
			}
			if tc.absent {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, 1, *calls)
		}
	}
}

func TestPortDiagnosticsDoNotLeak(t *testing.T) {
	r, _ := portFixture(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"message":"private server prose"}`)
	})
	_, err := r.Read(t.Context(), infer.ReadRequest[PortArgs, PortState]{ID: "secret-id"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-id")
	require.NotContains(t, err.Error(), "private server prose")
	require.Contains(t, err.Error(), "403")
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		require.ErrorIs(t, sanitizePortError(cause), cause)
	}
}

func TestPortCreateTransportFailureDoesNotRetry(t *testing.T) {
	called := 0
	r, _ := portFixture(t, func(w http.ResponseWriter, req *http.Request) {
		called++
		require.Equal(t, "/api/port.create", req.URL.Path)
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	})
	got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
	require.Error(t, err)
	require.Empty(t, got.ID)
	require.Equal(t, 1, called)
}

type portRoundTripFunc func(*http.Request) (*http.Response, error)

func (f portRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type portCloseErrorBody struct{ io.Reader }

func (portCloseErrorBody) Close() error { return errors.New("private close detail") }

func TestPortCreateIdentitySurvivesCloseError(t *testing.T) {
	calls := 0
	transport := portRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		status, body := 200, `{"portId":"p1","publishedPort":"invalid"}`
		if calls == 2 {
			require.Equal(t, "/api/port.one", req.URL.Path)
			status, body = 404, `{}`
		} else {
			require.Equal(t, 1, calls)
			require.Equal(t, "/api/port.create", req.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: portCloseErrorBody{strings.NewReader(body)}, Request: req}, nil
	})
	api, err := client.New("https://placeholder.example", "placeholder", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	got, err := (Port{client: fixedClient(api)}).Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
	var partial infer.ResourceInitFailedError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "p1", got.Output.PortID)
	require.Equal(t, 2, calls)
}

func TestPortTransportErrorsDoNotLeakURLs(t *testing.T) {
	for _, op := range []string{"create", "read", "update", "delete"} {
		t.Run(op, func(t *testing.T) {
			calls := 0
			transport := portRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("private transport URL https://private.example/internal?token=hidden")
			})
			api, err := client.New("https://private.example/internal", "placeholder", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			r := Port{client: fixedClient(api)}
			var failure error
			switch op {
			case "create":
				_, failure = r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: portArgs()})
			case "read":
				_, failure = r.Read(t.Context(), infer.ReadRequest[PortArgs, PortState]{ID: "secret-id"})
			case "update":
				_, failure = r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "secret-id", Inputs: portArgs(), State: PortState{PortArgs: portArgs()}})
			case "delete":
				_, failure = r.Delete(t.Context(), infer.DeleteRequest[PortState]{ID: "secret-id"})
			}
			require.Error(t, failure)
			for _, secret := range []string{"secret-id", "private.example", "token=hidden", "private transport URL"} {
				require.NotContains(t, failure.Error(), secret)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestPortCanceledAndDeadlineContexts(t *testing.T) {
	r := Port{client: func(context.Context) *client.Client {
		api, err := client.New("https://placeholder.example", "placeholder", client.WithHTTPClient(&http.Client{Transport: portRoundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, req.Context().Err() })}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
		require.NoError(t, err)
		return api
	}}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		ctx, cancel := context.WithCancel(t.Context())
		if cause == context.Canceled {
			cancel()
		} else {
			cancel()
			var deadlineCancel context.CancelFunc
			ctx, deadlineCancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			defer deadlineCancel()
		}
		_, err := r.Read(ctx, infer.ReadRequest[PortArgs, PortState]{ID: "p1"})
		require.ErrorIs(t, err, cause)
	}
}

func TestPortNilEnumInputsUseDefaultsOnWire(t *testing.T) {
	a := portArgs()
	a.Protocol = nil
	a.PublishMode = nil
	r, calls := portFixture(t, func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/api/port.create", req.URL.Path)
		var b map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
		require.Equal(t, "tcp", b["protocol"])
		require.Equal(t, "ingress", b["publishMode"])
		_, _ = io.WriteString(w, `{"portId":"p1"}`)
	}, portStep(t, "GET", "one", portRow))
	got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: a})
	require.NoError(t, err)
	require.Equal(t, ptr(PortProtocolTCP), got.Output.Protocol)
	require.Equal(t, ptr(PortPublishModeIngress), got.Output.PublishMode)
	require.Equal(t, 2, *calls)
}
