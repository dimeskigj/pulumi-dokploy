package dokploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
)

type scriptedRequest struct {
	Method   string
	Path     string
	Query    url.Values
	Body     any
	Status   int
	Response []byte
}

type scriptedServer struct {
	t            *testing.T
	server       *httptest.Server
	mu           sync.Mutex
	expectations []scriptedRequest
}

func TestScriptedServer(t *testing.T) {
	s := newScriptedServer(t, scriptedRequest{
		Method:   http.MethodGet,
		Path:     "/api/application.one",
		Query:    url.Values{"applicationId": {"a1"}},
		Status:   http.StatusOK,
		Response: []byte(`{}`),
	})
	response, err := s.API().ApplicationOneWithResponse(t.Context(), &generated.ApplicationOneParams{ApplicationId: "a1"})
	if err != nil {
		t.Fatalf("application one: %v", err)
	}
	if response.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode(), http.StatusOK)
	}
}

func newScriptedServer(t *testing.T, expectations ...scriptedRequest) *scriptedServer {
	t.Helper()
	s := &scriptedServer{t: t, expectations: append([]scriptedRequest(nil), expectations...)}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(func() {
		s.mu.Lock()
		remaining := len(s.expectations)
		s.mu.Unlock()
		if remaining != 0 {
			t.Errorf("scripted server has %d expected request(s) remaining", remaining)
		}
		s.server.Close()
	})
	return s
}

func (s *scriptedServer) API() *client.Client {
	s.t.Helper()
	api, err := client.New(s.server.URL, "test-api-key")
	if err != nil {
		s.t.Fatalf("create test API client: %v", err)
	}
	return api
}

func (s *scriptedServer) handle(w http.ResponseWriter, r *http.Request) {
	s.t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read request body: %v", err)
		return
	}
	s.mu.Lock()
	if len(s.expectations) == 0 {
		s.mu.Unlock()
		s.t.Errorf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
		return
	}
	expectation := s.expectations[0]
	requestNumber := len(s.expectations)
	s.expectations = s.expectations[1:]
	s.mu.Unlock()

	if r.Method != expectation.Method {
		s.t.Errorf("request %d method mismatch: got %q, want %q", requestNumber, r.Method, expectation.Method)
	}
	if r.URL.Path != expectation.Path {
		s.t.Errorf("request path mismatch: got %q, want %q", r.URL.Path, expectation.Path)
	}
	if !sameQuery(r.URL.Query(), expectation.Query) {
		s.t.Errorf("request query mismatch: got %v, want %v", r.URL.Query(), expectation.Query)
	}
	if expectation.Body != nil {
		var got, want any
		if err := json.Unmarshal(body, &got); err != nil {
			s.t.Errorf("request body is not JSON: %v", err)
		} else if err := json.Unmarshal(mustJSON(expectation.Body), &want); err != nil {
			s.t.Errorf("expected body is not JSON: %v", err)
		} else if !reflect.DeepEqual(got, want) {
			s.t.Errorf("request body mismatch: got %s, want %s", body, mustJSON(expectation.Body))
		}
	} else if len(bytes.TrimSpace(body)) != 0 {
		s.t.Errorf("request body mismatch: got %s, want empty", body)
	}

	w.WriteHeader(expectation.Status)
	_, _ = w.Write(expectation.Response)
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal expected JSON: %v", err))
	}
	return data
}

func sameQuery(got, want url.Values) bool {
	if len(got) != len(want) {
		return false
	}
	return reflect.DeepEqual(got, want)
}
