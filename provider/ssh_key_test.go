package dokploy

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestSSHKeyCheckValidatesRequiredFieldsAndAllowsComputedValues(t *testing.T) {
	got, err := (SSHKey{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{})})
	require.NoError(t, err)
	require.Len(t, got.Failures, 3)

	computed := property.NewMap(map[string]property.Value{
		"name":       property.New(property.Computed),
		"privateKey": property.New(property.Computed),
		"publicKey":  property.New(property.Computed),
	})
	got, err = (SSHKey{}).Check(t.Context(), infer.CheckRequest{NewInputs: computed})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}

func TestSSHKeyDiffReplacesKeysAndUpdatesMetadata(t *testing.T) {
	diff, err := (SSHKey{}).Diff(t.Context(), infer.DiffRequest[SSHKeyArgs, SSHKeyState]{
		Inputs: SSHKeyArgs{Name: "new", PrivateKey: "private-2", PublicKey: "public-2"},
		State:  SSHKeyState{SSHKeyArgs: SSHKeyArgs{Name: "old", PrivateKey: "private-1", PublicKey: "public-1"}},
	})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["name"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["privateKey"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["publicKey"].Kind)
}

func TestSSHKeyCreateReadsAndUpdatesInOrder(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"id":"org1"}`),
		expectPOST("/api/sshKey.create", `{"name":"key","description":"desc","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{}`),
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"k1","name":"key"}]`),
		expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"k1"}}, http.StatusOK, `{"sshKeyId":"k1","name":"key","description":"desc","organizationId":"org1","privateKey":"private","publicKey":"public"}`),
		expectPOST("/api/sshKey.update", `{"sshKeyId":"k1","name":"key2","description":"updated"}`, `{}`),
		expectPOST("/api/sshKey.remove", `{"sshKeyId":"k1"}`, `{}`),
	)
	r := SSHKey{client: fixedClient(s.API())}
	description := "desc"
	created, err := r.Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", Description: &description, PrivateKey: "private", PublicKey: "public"}})
	require.NoError(t, err)
	require.Equal(t, "k1", created.ID)
	require.Equal(t, "org1", created.Output.OrganizationID)
	updatedDescription := "updated"
	_, err = r.Update(t.Context(), infer.UpdateRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", Inputs: SSHKeyArgs{Name: "key2", Description: &updatedDescription, PrivateKey: "private", PublicKey: "public"}, State: created.Output})
	require.NoError(t, err)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[SSHKeyState]{ID: "k1"})
	require.NoError(t, err)
}

func TestSSHKeyCreateFailsInitializationWhenPostCreateReadIsGone(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		expectPOST("/api/sshKey.create", `{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{}`),
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"k1","name":"key"}]`),
		scriptedRequest{Method: http.MethodGet, Path: "/api/sshKey.one", Query: map[string][]string{"sshKeyId": {"k1"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	created, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	var initErr infer.ResourceInitFailedError
	require.True(t, errors.As(err, &initErr))
	require.Equal(t, "k1", created.ID)
	require.Equal(t, "org1", created.Output.OrganizationID)
	require.Equal(t, "private", created.Output.PrivateKey)
}

func TestSSHKeyCreateRejectsIncompleteOrganization(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{}`),
	)
	_, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	require.EqualError(t, err, "organization.active returned incomplete organization")
}

func TestSSHKeyCreateRejectsWhenEmptyCreateHasNoCandidate(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		expectPOST("/api/sshKey.create", `{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{}`),
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
	)
	_, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	var initErr infer.ResourceInitFailedError
	require.ErrorAs(t, err, &initErr)
	require.Equal(t, []string{"sshKey.create returned no discoverable SSH key"}, initErr.Reasons)
}

func TestSSHKeyCreateRejectsWhenEmptyCreateHasAmbiguousCandidates(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		expectPOST("/api/sshKey.create", `{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{}`),
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"k1","name":"key"},{"sshKeyId":"k2","name":"key"}]`),
	)
	_, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	var initErr infer.ResourceInitFailedError
	require.ErrorAs(t, err, &initErr)
	require.Equal(t, []string{"sshKey.create returned ambiguous SSH key discovery"}, initErr.Reasons)
}

func TestSSHKeyCreateExcludesPreexistingSameNameFromDiscovery(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"old","name":"key"}]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		expectPOST("/api/sshKey.create", `{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{}`),
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"old","name":"key"},{"sshKeyId":"new","name":"key"}]`),
		expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"new"}}, http.StatusOK, `{"sshKeyId":"new","name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`),
	)
	created, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	require.NoError(t, err)
	require.Equal(t, "new", created.ID)
}

func TestSSHKeyCreateRecoversPersistedKeyAfterCreateError(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/sshKey.create", Body: json.RawMessage(`{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"create private public"}`)},
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"new","name":"key"}]`),
		expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"new"}}, http.StatusOK, `{"sshKeyId":"new","name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`),
	)
	created, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	var initErr infer.ResourceInitFailedError
	require.ErrorAs(t, err, &initErr)
	require.Equal(t, "new", created.ID)
	require.Equal(t, "org1", created.Output.OrganizationID)
	require.NotContains(t, err.Error(), "private")
	require.NotContains(t, err.Error(), "public")
}

func TestSSHKeyCreateReturnsOriginalErrorWhenPostCreateDiscoveryFindsNothing(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/sshKey.create", Body: json.RawMessage(`{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"create private public"}`)},
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
	)
	_, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private")
	require.NotContains(t, err.Error(), "public")
	require.NotContains(t, err.Error(), "discovery")
}

func TestSSHKeyCreateReturnsAmbiguousInitializationWithoutIDAfterCreateError(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[]`),
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/sshKey.create", Body: json.RawMessage(`{"description":null,"name":"key","organizationId":"org1","privateKey":"private","publicKey":"public"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"create private public"}`)},
		expectGET("/api/sshKey.all", nil, http.StatusOK, `[{"sshKeyId":"one","name":"key"},{"sshKeyId":"two","name":"key"}]`),
	)
	created, err := (SSHKey{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	var initErr infer.ResourceInitFailedError
	require.ErrorAs(t, err, &initErr)
	require.Empty(t, created.ID)
	require.Equal(t, []string{"sshKey.create returned ambiguous SSH key discovery"}, initErr.Reasons)
}

func TestActiveOrganizationIDPrefersIDAndSupportsLegacyOrganizationID(t *testing.T) {
	for _, test := range []struct {
		name string
		org  generated.Organization
		want string
	}{
		{name: "primary", org: generated.Organization{Id: stringPtr("primary"), OrganizationId: stringPtr("legacy")}, want: "primary"},
		{name: "legacy", org: generated.Organization{OrganizationId: stringPtr("legacy")}, want: "legacy"},
		{name: "incomplete", org: generated.Organization{}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, activeOrganizationID(test.org))
		})
	}
}

func TestSSHKeyReadPreservesPrivateKeyWhenOmitted(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"k1"}}, http.StatusOK, `{"sshKeyId":"k1","name":"key","organizationId":"org1","publicKey":"public"}`))
	read, err := (SSHKey{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", State: SSHKeyState{SSHKeyArgs: SSHKeyArgs{PrivateKey: "prior-private"}}})
	require.NoError(t, err)
	require.Equal(t, "prior-private", read.Inputs.PrivateKey)
}

func TestSSHKeyReadSupportsImport(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"k1"}}, http.StatusOK, `{"sshKeyId":"k1","name":"imported","description":"desc","organizationId":"org1","privateKey":"private","publicKey":"public"}`))
	read, err := (SSHKey{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: "k1"})
	require.NoError(t, err)
	require.Equal(t, "k1", read.State.SSHKeyID)
	require.Equal(t, "org1", read.State.OrganizationID)
	require.Equal(t, "imported", read.Inputs.Name)
	require.Equal(t, "desc", *read.Inputs.Description)
}

func TestSSHKeyUpdateClearsDescription(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/sshKey.update", `{"sshKeyId":"k1","name":"key","description":null}`, `{}`))
	_, err := (SSHKey{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", Inputs: SSHKeyArgs{Name: "key", PrivateKey: "private", PublicKey: "public"}})
	require.NoError(t, err)
}

func TestSSHKeyAPIErrorsRedactCurrentAndPriorSecrets(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
		call   func(SSHKey, SSHKeyArgs, SSHKeyState) error
	}{
		{"create", http.MethodPost, "/api/sshKey.create", `{"description":null,"name":"key","organizationId":"org1","privateKey":"current-private","publicKey":"current-public"}`, func(r SSHKey, a SSHKeyArgs, _ SSHKeyState) error {
			_, err := r.Create(t.Context(), infer.CreateRequest[SSHKeyArgs]{Inputs: a})
			return err
		}},
		{"update", http.MethodPost, "/api/sshKey.update", `{"sshKeyId":"k1","name":"key","description":null}`, func(r SSHKey, a SSHKeyArgs, s SSHKeyState) error {
			_, err := r.Update(t.Context(), infer.UpdateRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", Inputs: a, State: s})
			return err
		}},
		{"delete", http.MethodPost, "/api/sshKey.remove", `{"sshKeyId":"k1"}`, func(r SSHKey, _ SSHKeyArgs, s SSHKeyState) error {
			_, err := r.Delete(t.Context(), infer.DeleteRequest[SSHKeyState]{ID: "k1", State: s})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			expectations := []scriptedRequest{{Method: test.method, Path: test.path, Body: json.RawMessage(test.body), Status: http.StatusBadRequest, Response: []byte(`{"message":"current-private current-public prior-private prior-public"}`)}}
			if test.name == "create" {
				expectations = append([]scriptedRequest{
					{Method: http.MethodGet, Path: "/api/sshKey.all", Status: http.StatusOK, Response: []byte(`[]`)},
					{Method: http.MethodGet, Path: "/api/organization.active", Status: http.StatusOK, Response: []byte(`{"organizationId":"org1"}`)},
				}, expectations...)
				expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/sshKey.all", Status: http.StatusOK, Response: []byte(`[]`)})
			}
			s := newScriptedServer(t, expectations...)
			err := test.call(SSHKey{client: fixedClient(s.API())}, SSHKeyArgs{Name: "key", PrivateKey: "current-private", PublicKey: "current-public"}, SSHKeyState{SSHKeyArgs: SSHKeyArgs{Name: "old", PrivateKey: "prior-private", PublicKey: "prior-public"}})
			require.Error(t, err)
			for _, secret := range []string{"current-private", "current-public", "prior-private", "prior-public"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestSSHKeyReadAndDeleteTreatNotFoundAsGone(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodGet, Path: "/api/sshKey.one", Query: map[string][]string{"sshKeyId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/sshKey.remove", Body: json.RawMessage(`{"sshKeyId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := SSHKey{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[SSHKeyState]{ID: "missing"})
	require.NoError(t, err)
}

func TestSSHKeyReadRedactsCurrentAndPriorSecrets(t *testing.T) {
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/sshKey.one", Query: map[string][]string{"sshKeyId": {"k1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"current-private current-public prior-private prior-public"}`)})
	_, err := (SSHKey{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", State: SSHKeyState{SSHKeyArgs: SSHKeyArgs{PrivateKey: "prior-private", PublicKey: "prior-public"}}})
	require.Error(t, err)
	for _, secret := range []string{"current-private", "current-public", "prior-private", "prior-public"} {
		require.NotContains(t, err.Error(), secret)
	}
}

func TestSSHKeyProviderRegistration(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:SSHKey")
	require.True(t, spec.Resources["dokploy:index:SSHKey"].InputProperties["privateKey"].Secret)
}
