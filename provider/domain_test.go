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

func TestDomainTargetValidation(t *testing.T) {
	for name, args := range map[string]DomainArgs{
		"none":                     {Host: "app.example.com"},
		"both":                     {ApplicationID: stringPtr("a1"), ComposeID: stringPtr("c1"), Host: "app.example.com"},
		"compose without service":  {ComposeID: stringPtr("c1"), Host: "app.example.com"},
		"service with application": {ApplicationID: stringPtr("a1"), ServiceName: stringPtr("web"), Host: "app.example.com"},
		"custom without resolver":  {ApplicationID: stringPtr("a1"), CertificateType: CertificateCustom, Host: "app.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			r := Domain{}
			checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
				"host":            property.New(args.Host),
				"applicationId":   optionalStringProperty(args.ApplicationID),
				"composeId":       optionalStringProperty(args.ComposeID),
				"serviceName":     optionalStringProperty(args.ServiceName),
				"certificateType": property.New(string(args.CertificateType)),
			})})
			require.NoError(t, err)
			require.NotEmpty(t, checked.Failures)
		})
	}
}

func optionalStringProperty(v *string) property.Value {
	if v == nil {
		return property.New("")
	}
	return property.New(*v)
}

func TestDomainCheckDefaults(t *testing.T) {
	checked, err := (Domain{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"applicationId": property.New("a1"), "host": property.New("app.example.com"),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.True(t, checked.Inputs.HTTPS)
	require.Equal(t, CertificateLetsencrypt, checked.Inputs.CertificateType)
	require.False(t, checked.Inputs.StripPath)
	require.True(t, checked.Inputs.Enabled)
}

func TestDomainCheckPreservesExplicitDisabled(t *testing.T) {
	checked, err := (Domain{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"applicationId": property.New("a1"), "host": property.New("app.example.com"), "enabled": property.New(false),
	})})
	require.NoError(t, err)
	require.False(t, checked.Inputs.Enabled)
}

func TestDomainDiff(t *testing.T) {
	old := DomainArgs{ApplicationID: stringPtr("a1"), Host: "old.example.com", Path: stringPtr("/")}
	in := DomainArgs{ApplicationID: stringPtr("a1"), Host: "new.example.com", Path: stringPtr("/api"), HTTPS: true}
	d, err := (Domain{}).Diff(t.Context(), infer.DiffRequest[DomainArgs, DomainState]{Inputs: in, State: DomainState{DomainArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.Update, d.DetailedDiff["host"].Kind)
	require.Equal(t, p.Update, d.DetailedDiff["path"].Kind)
	require.NotContains(t, d.DetailedDiff, "applicationId")
	old.ComposeID = stringPtr("c1")
	in.ComposeID = stringPtr("c2")
	d, err = (Domain{}).Diff(t.Context(), infer.DiffRequest[DomainArgs, DomainState]{Inputs: in, State: DomainState{DomainArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, d.DetailedDiff["composeId"].Kind)
}

func TestDomainCreateApplicationAndDisabledUpdate(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/domain.create", `{"applicationId":"a1","certificateType":"letsencrypt","domainType":"application","host":"app.example.com","https":true,"stripPath":false}`, `{"domainId":"d1"}`),
		expectPOST("/api/domain.update", `{"certificateType":"letsencrypt","customCertResolver":null,"domainId":"d1","domainType":"application","host":"app.example.com","https":true,"internalPath":null,"path":null,"port":null,"serviceName":null,"stripPath":false}`, `{}`),
	)
	r := Domain{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{ApplicationID: stringPtr("a1"), Host: "app.example.com", Enabled: false, HTTPS: true, CertificateType: CertificateLetsencrypt}})
	require.NoError(t, err)
	require.Equal(t, "d1", got.ID)
}

func TestDomainCreateComposePreviewAndRoutingUpdate(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/domain.create", `{"certificateType":"letsencrypt","composeId":"c1","domainType":"compose","host":"app.example.com","https":true,"serviceName":"web","stripPath":false}`, `{"domainId":"d1"}`),
		expectPOST("/api/domain.update", `{"certificateType":"letsencrypt","customCertResolver":null,"domainId":"d1","domainType":"compose","host":"app.example.com","https":true,"internalPath":null,"path":"/api","port":8080,"serviceName":"web","stripPath":false}`, `{}`),
	)
	r := Domain{client: fixedClient(s.API())}
	_, err := r.Create(t.Context(), infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{ComposeID: stringPtr("c1"), ServiceName: stringPtr("web"), Host: "app.example.com", HTTPS: true, StripPath: false, Enabled: true, CertificateType: ""}})
	require.NoError(t, err)
	_, err = r.Update(t.Context(), infer.UpdateRequest[DomainArgs, DomainState]{ID: "d1", Inputs: DomainArgs{ComposeID: stringPtr("c1"), ServiceName: stringPtr("web"), Host: "app.example.com", Path: stringPtr("/api"), Port: intPtr(8080), HTTPS: true, CertificateType: CertificateLetsencrypt}, State: DomainState{DomainID: "d1"}})
	require.NoError(t, err)
	_, err = r.Create(t.Context(), infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{ApplicationID: stringPtr("a1"), Host: "preview.example.com"}, DryRun: true})
	require.NoError(t, err)
}

func TestDomainCreateReturnsPartialStateWhenDisabledUpdateFails(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/domain.create", `{"applicationId":"a1","certificateType":"letsencrypt","domainType":"application","host":"app.example.com","https":true,"stripPath":false}`, `{"domainId":"d1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/domain.update", Body: json.RawMessage(`{"certificateType":"letsencrypt","customCertResolver":null,"domainId":"d1","domainType":"application","host":"app.example.com","https":true,"internalPath":null,"path":null,"port":null,"serviceName":null,"stripPath":false}`), Status: http.StatusInternalServerError, Response: []byte(`{"message":"failed"}`)},
	)
	r := Domain{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{ApplicationID: stringPtr("a1"), Host: "app.example.com", Enabled: false, HTTPS: true}})
	require.Error(t, err)
	require.Equal(t, "d1", got.ID)
	require.Equal(t, "d1", got.Output.DomainID)
}

func TestDomainReadImportAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/domain.one", map[string][]string{"domainId": {"d1"}}, http.StatusOK, `{"domainId":"d1","applicationId":"a1","host":"app.example.com","path":"/api","internalPath":"/","port":8080,"https":true,"certificateType":"custom","customCertResolver":"r1","stripPath":true,"enabled":false}`),
		expectGET("/api/domain.one", map[string][]string{"domainId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/domain.delete", Body: json.RawMessage(`{"domainId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Domain{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[DomainArgs, DomainState]{ID: "d1"})
	require.NoError(t, err)
	require.Equal(t, "a1", *read.Inputs.ApplicationID)
	require.Equal(t, 8080, *read.Inputs.Port)
	require.False(t, read.Inputs.Enabled)
	read, err = r.Read(t.Context(), infer.ReadRequest[DomainArgs, DomainState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[DomainState]{ID: "missing"})
	require.NoError(t, err)
}

func intPtr(v int) *int { return &v }

func TestDomainProviderRegistration(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:Domain")
}
