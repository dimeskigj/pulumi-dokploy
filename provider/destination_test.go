package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestDestinationCheckDefaultsProviderAndValidatesRequiredFields(t *testing.T) {
	got, err := (Destination{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("dest"), "accessKey": property.New("key"), "secretAccessKey": property.New("secret"),
		"bucket": property.New("bucket"), "region": property.New("us-east-1"), "endpoint": property.New("https://s3.example.com"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	require.Equal(t, "s3", *got.Inputs.Provider)

	empty, err := (Destination{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{})})
	require.NoError(t, err)
	require.Len(t, empty.Failures, 6)
}

func TestDestinationDiff(t *testing.T) {
	old := DestinationArgs{Name: "dest", AccessKey: "key", Bucket: "b1"}
	in := DestinationArgs{Name: "dest2", AccessKey: "key", Bucket: "b2", AdditionalFlags: []string{"--fast-list"}}
	d, err := (Destination{}).Diff(t.Context(), infer.DiffRequest[DestinationArgs, DestinationState]{Inputs: in, State: DestinationState{DestinationArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.Update, d.DetailedDiff["name"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["bucket"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["additionalFlags"].Kind)
	require.NotContains(t, d.DetailedDiff, "accessKey")
	unchanged, err := (Destination{}).Diff(t.Context(), infer.DiffRequest[DestinationArgs, DestinationState]{Inputs: in, State: DestinationState{DestinationArgs: in}})
	require.NoError(t, err)
	require.Empty(t, unchanged.DetailedDiff)
}

func TestDestinationCreateAndRead(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/destination.create",
			`{"name":"dest","provider":"s3","accessKey":"key","secretAccessKey":"secret","bucket":"bucket","region":"us-east-1","endpoint":"https://s3.example.com","additionalFlags":["--fast-list"],"serverId":"srv1"}`,
			`{"destinationId":"d1"}`),
		expectGET("/api/destination.one", map[string][]string{"destinationId": {"d1"}}, http.StatusOK,
			`{"destinationId":"d1","name":"dest","provider":"s3","accessKey":"key","secretAccessKey":"secret","bucket":"bucket","region":"us-east-1","endpoint":"https://s3.example.com","additionalFlags":["--fast-list"],"serverId":"srv1"}`),
	)
	r := Destination{client: fixedClient(s.API())}
	server := "srv1"
	got, err := r.Create(t.Context(), infer.CreateRequest[DestinationArgs]{Inputs: DestinationArgs{
		Name: "dest", Provider: stringPtr("s3"), AccessKey: "key", SecretAccessKey: "secret", Bucket: "bucket",
		Region: "us-east-1", Endpoint: "https://s3.example.com", AdditionalFlags: []string{"--fast-list"}, ServerID: &server,
	}})
	require.NoError(t, err)
	require.Equal(t, "d1", got.ID)

	read, err := r.Read(t.Context(), infer.ReadRequest[DestinationArgs, DestinationState]{ID: "d1"})
	require.NoError(t, err)
	require.Equal(t, "secret", read.Inputs.SecretAccessKey)
	require.Equal(t, []string{"--fast-list"}, read.Inputs.AdditionalFlags)
	require.Equal(t, "srv1", *read.Inputs.ServerID)
}

func TestDestinationCreateRedactsSecretOnFailure(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/destination.create", Body: json.RawMessage(`{"name":"dest","provider":"s3","accessKey":"key","secretAccessKey":"SUPER-SECRET","bucket":"bucket","region":"us-east-1","endpoint":"https://s3.example.com","additionalFlags":null}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"failed SUPER-SECRET"}`)},
	)
	_, err := (Destination{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[DestinationArgs]{Inputs: DestinationArgs{
		Name: "dest", Provider: stringPtr("s3"), AccessKey: "key", SecretAccessKey: "SUPER-SECRET", Bucket: "bucket",
		Region: "us-east-1", Endpoint: "https://s3.example.com",
	}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SUPER-SECRET")
}

func TestDestinationReadPreservesSecretWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/destination.one", map[string][]string{"destinationId": {"d1"}}, http.StatusOK,
		`{"destinationId":"d1","name":"dest","accessKey":"key","bucket":"bucket","region":"us-east-1","endpoint":"https://s3.example.com"}`))
	r := Destination{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[DestinationArgs, DestinationState]{ID: "d1", State: DestinationState{DestinationArgs: DestinationArgs{SecretAccessKey: "prior-secret"}}})
	require.NoError(t, err)
	require.Equal(t, "prior-secret", read.Inputs.SecretAccessKey)
}

func TestDestinationUpdate(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/destination.update",
			`{"destinationId":"d1","name":"dest2","provider":"s3","accessKey":"key","secretAccessKey":"secret","bucket":"bucket","region":"us-east-1","endpoint":"https://s3.example.com","additionalFlags":null}`,
			`{"destinationId":"d1"}`),
	)
	r := Destination{client: fixedClient(s.API())}
	_, err := r.Update(t.Context(), infer.UpdateRequest[DestinationArgs, DestinationState]{ID: "d1", Inputs: DestinationArgs{
		Name: "dest2", Provider: stringPtr("s3"), AccessKey: "key", SecretAccessKey: "secret", Bucket: "bucket",
		Region: "us-east-1", Endpoint: "https://s3.example.com",
	}})
	require.NoError(t, err)
}

func TestDestinationReadNotFoundAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/destination.one", map[string][]string{"destinationId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/destination.remove", Body: json.RawMessage(`{"destinationId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Destination{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[DestinationArgs, DestinationState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[DestinationState]{ID: "missing"})
	require.NoError(t, err)
}

func TestDestinationProviderRegistration(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:Destination")
	require.True(t, spec.Resources["dokploy:index:Destination"].InputProperties["secretAccessKey"].Secret)
}
