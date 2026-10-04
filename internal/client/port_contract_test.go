package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestPortGeneratedClientContract(t *testing.T) {
	var requests []struct {
		Path string
		Body map[string]any
	}
	var queriedPortID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			requests = append(requests, struct {
				Path string
				Body map[string]any
			}{r.URL.Path, body})
			_, _ = w.Write([]byte(`{"portId":"ack"}`))
			return
		}
		queriedPortID = r.URL.Query().Get("portId")
		_, _ = fmt.Fprint(w, `{"portId":"opaque","publishedPort":8080,"targetPort":80,"application":{"applicationId":"ignored"}}`)
	}))
	defer server.Close()
	c, err := New(server.URL, "test-key")
	require.NoError(t, err)
	protocol := generated.PortCreateJSONBodyProtocol("udp")
	mode := generated.PortCreateJSONBodyPublishMode("host")
	_, err = c.PortCreateWithResponse(t.Context(), generated.PortCreateJSONRequestBody{ApplicationId: "app", PublishedPort: 8080, TargetPort: 80, Protocol: protocol, PublishMode: mode})
	require.NoError(t, err)
	updateProtocol := generated.PortUpdateJSONBodyProtocol("tcp")
	updateMode := generated.PortUpdateJSONBodyPublishMode("ingress")
	_, err = c.PortUpdateWithResponse(t.Context(), generated.PortUpdateJSONRequestBody{PortId: "opaque", PublishedPort: 8081, TargetPort: 81, Protocol: updateProtocol, PublishMode: updateMode})
	require.NoError(t, err)
	require.Len(t, requests, 2)
	require.Equal(t, map[string]any{"applicationId": "app", "publishedPort": float64(8080), "targetPort": float64(80), "protocol": "udp", "publishMode": "host"}, requests[0].Body)
	require.Equal(t, map[string]any{"portId": "opaque", "publishedPort": float64(8081), "targetPort": float64(81), "protocol": "tcp", "publishMode": "ingress"}, requests[1].Body)
	require.NotContains(t, requests[1].Body, "applicationId")

	response, err := c.PortOneWithResponse(t.Context(), &generated.PortOneParams{PortId: `id /?&`})
	require.NoError(t, err)
	require.Equal(t, `id /?&`, queriedPortID)
	require.Equal(t, 8080, *response.JSON200.PublishedPort)
	require.Equal(t, 80, *response.JSON200.TargetPort)
	require.Nil(t, response.JSON200.ApplicationId)
	require.Nil(t, response.JSON200.Protocol)
	require.Nil(t, response.JSON200.PublishMode)
}

func TestPortResponseIntegerDecoding(t *testing.T) {
	for _, tc := range []struct {
		body      string
		wantError bool
	}{
		{`{"portId":"p","publishedPort":1,"targetPort":2}`, false},
		{`{"portId":"p","publishedPort":1.5,"targetPort":2}`, true},
		{`{"portId":"p","applicationId":null,"publishedPort":null,"targetPort":null,"protocol":null,"publishMode":null}`, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, tc.body)
		}))
		c, err := New(server.URL, "test-key")
		require.NoError(t, err)
		response, err := c.PortOneWithResponse(t.Context(), &generated.PortOneParams{PortId: "opaque"})
		require.Equal(t, tc.wantError, err != nil)
		if !tc.wantError {
			require.NoError(t, err)
			if tc.body == `{"portId":"p","applicationId":null,"publishedPort":null,"targetPort":null,"protocol":null,"publishMode":null}` {
				require.Nil(t, response.JSON200.ApplicationId)
				require.Nil(t, response.JSON200.PublishedPort)
				require.Nil(t, response.JSON200.TargetPort)
				require.Nil(t, response.JSON200.Protocol)
				require.Nil(t, response.JSON200.PublishMode)
			}
		}
		server.Close()
	}
}
