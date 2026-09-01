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
		expectGET("/api/organization.active", nil, http.StatusOK, `{"organizationId":"org1"}`),
		expectPOST("/api/sshKey.create", `{"name":"key","description":"desc","organizationId":"org1","privateKey":"private","publicKey":"public"}`, `{"sshKeyId":"k1"}`),
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

func TestSSHKeyReadPreservesPrivateKeyWhenOmitted(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/sshKey.one", map[string][]string{"sshKeyId": {"k1"}}, http.StatusOK, `{"sshKeyId":"k1","name":"key","organizationId":"org1","publicKey":"public"}`))
	read, err := (SSHKey{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: "k1", State: SSHKeyState{SSHKeyArgs: SSHKeyArgs{PrivateKey: "prior-private"}}})
	require.NoError(t, err)
	require.Equal(t, "prior-private", read.Inputs.PrivateKey)
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
