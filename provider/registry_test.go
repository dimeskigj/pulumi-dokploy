package dokploy

import (
	"errors"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestRegistryCheckValidatesRequiredFields(t *testing.T) {
	got, err := (Registry{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("r"), "username": property.New("u"), "password": property.New("p"), "url": property.New("https://registry.example.com"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	empty, err := (Registry{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{})})
	require.NoError(t, err)
	require.Len(t, empty.Failures, 4)
}

func TestRegistryDiffReportsAllUpdatesExceptNameOnly(t *testing.T) {
	oldPrefix, oldServer := "old/", "old-server"
	newPrefix, newServer := "new/", "new-server"
	old := RegistryArgs{Name: "old", Username: "old-user", Password: "old-pass", URL: "old-url", ImagePrefix: &oldPrefix, ServerID: &oldServer}
	in := RegistryArgs{Name: "new", Username: "new-user", Password: "new-pass", URL: "new-url", ImagePrefix: &newPrefix, ServerID: &newServer}
	d, err := (Registry{}).Diff(t.Context(), infer.DiffRequest[RegistryArgs, RegistryState]{Inputs: in, State: RegistryState{RegistryArgs: old}})
	require.NoError(t, err)
	for _, field := range []string{"name", "username", "password", "url", "imagePrefix", "serverId"} {
		require.Equal(t, p.Update, d.DetailedDiff[field].Kind, field)
	}
	nameOnly, err := (Registry{}).Diff(t.Context(), infer.DiffRequest[RegistryArgs, RegistryState]{Inputs: RegistryArgs{Name: "new", Username: "old-user", Password: "old-pass", URL: "old-url"}, State: RegistryState{RegistryArgs: RegistryArgs{Name: "old", Username: "old-user", Password: "old-pass", URL: "old-url"}}})
	require.NoError(t, err)
	require.Equal(t, p.Update, nameOnly.DetailedDiff["name"].Kind)
}

func TestRegistryCreateTestsCredentialsThenCreates(t *testing.T) {
	prefix, server := "team/", "srv1"
	s := newScriptedServer(t,
		expectPOST("/api/registry.testRegistry", `{"password":"secret","registryType":"cloud","registryUrl":"https://registry.example.com","username":"user","imagePrefix":"team/","serverId":"srv1"}`, `{}`),
		expectPOST("/api/registry.create", `{"imagePrefix":"team/","password":"secret","registryName":"reg","registryType":"cloud","registryUrl":"https://registry.example.com","serverId":"srv1","username":"user"}`, `{"registryId":"r1"}`),
	)
	got, err := (Registry{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[RegistryArgs]{Inputs: RegistryArgs{Name: "reg", Username: "user", Password: "secret", URL: "https://registry.example.com", ImagePrefix: &prefix, ServerID: &server}})
	require.NoError(t, err)
	require.Equal(t, "r1", got.ID)
}

func TestRegistryUpdateTestsChangedCredentialsAndConnection(t *testing.T) {
	prefix, server := "new/", "srv2"
	s := newScriptedServer(t,
		expectPOST("/api/registry.testRegistry", `{"password":"new-pass","registryType":"cloud","registryUrl":"new-url","username":"new-user","imagePrefix":"new/","serverId":"srv2"}`, `{}`),
		expectPOST("/api/registry.update", `{"imagePrefix":"new/","password":"new-pass","registryId":"r1","registryName":"new-name","registryType":"cloud","registryUrl":"new-url","serverId":"srv2","username":"new-user"}`, `{}`),
	)
	_, err := (Registry{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RegistryArgs, RegistryState]{ID: "r1", Inputs: RegistryArgs{Name: "new-name", Username: "new-user", Password: "new-pass", URL: "new-url", ImagePrefix: &prefix, ServerID: &server}})
	require.NoError(t, err)
}

func TestRegistryUpdateClearsOptionalFields(t *testing.T) {
	oldPrefix, oldServer := "old/", "old-server"
	s := newScriptedServer(t,
		expectPOST("/api/registry.testRegistry", `{"imagePrefix":null,"password":"pass","registryType":"cloud","registryUrl":"url","username":"user"}`, `{}`),
		expectPOST("/api/registry.update", `{"imagePrefix":null,"password":"pass","registryId":"r1","registryName":"reg","registryType":"cloud","registryUrl":"url","serverId":null,"username":"user"}`, `{}`),
	)
	_, err := (Registry{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RegistryArgs, RegistryState]{ID: "r1", Inputs: RegistryArgs{Name: "reg", Username: "user", Password: "pass", URL: "url"}, State: RegistryState{RegistryArgs: RegistryArgs{Name: "reg", Username: "user", Password: "pass", URL: "url", ImagePrefix: &oldPrefix, ServerID: &oldServer}}})
	require.NoError(t, err)
}

func TestRegistryNameOnlyUpdateSkipsCredentialTest(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/registry.update", `{"imagePrefix":null,"password":"pass","registryId":"r1","registryName":"new-name","registryType":"cloud","registryUrl":"url","serverId":null,"username":"user"}`, `{}`))
	_, err := (Registry{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RegistryArgs, RegistryState]{ID: "r1", Inputs: RegistryArgs{Name: "new-name", Username: "user", Password: "pass", URL: "url"}, State: RegistryState{RegistryArgs: RegistryArgs{Name: "old-name", Username: "user", Password: "pass", URL: "url"}}})
	require.NoError(t, err)
}

func TestRegistryReadPreservesPasswordAndHandlesNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/registry.one", map[string][]string{"registryId": {"r1"}}, http.StatusOK, `{"registryId":"r1","registryName":"reg","registryUrl":"url","username":"user"}`),
		expectGET("/api/registry.one", map[string][]string{"registryId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
	)
	r := Registry{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[RegistryArgs, RegistryState]{ID: "r1", State: RegistryState{RegistryArgs: RegistryArgs{Password: "prior"}}})
	require.NoError(t, err)
	require.Equal(t, "prior", read.Inputs.Password)
	missing, err := r.Read(t.Context(), infer.ReadRequest[RegistryArgs, RegistryState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, missing.ID)
}

func TestRegistrySanitizesCurrentAndPriorPasswords(t *testing.T) {
	err := sanitizeRegistryError(errors.New("failed current prior"), RegistryArgs{Password: "current"}, RegistryArgs{Password: "prior"})
	require.NotContains(t, err.Error(), "current")
	require.NotContains(t, err.Error(), "prior")
}

func TestRegistryProviderRegistrationAndSecret(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:Registry")
	require.True(t, spec.Resources["dokploy:index:Registry"].InputProperties["password"].Secret)
}
