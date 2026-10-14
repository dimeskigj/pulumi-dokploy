package dokploy

import (
	"context"
	"reflect"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestGiteaIntegrationCheck(t *testing.T) {
	checked, err := (GiteaIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("gitea"), "giteaUrl": property.New("https://gitea.example/path"),
		"clientId": property.New("client"), "clientSecret": property.New("secret"),
		"redirectUri": property.New("https://example.com/callback"),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.Empty(t, checked.Inputs.OrganizationName)
	require.Equal(t, "https://gitea.example/path", checked.Inputs.GiteaURL)
	require.Nil(t, checked.Inputs.GiteaInternalURL)

	computed, err := (GiteaIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New(""), "giteaUrl": property.New("not-url"),
		"clientId": property.New(property.Computed), "clientSecret": property.New(property.Computed),
		"redirectUri": property.New("https://example.com/callback"),
	})})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"name", "giteaUrl"}, []string{computed.Failures[0].Property, computed.Failures[1].Property})

	typ := reflect.TypeOf(GiteaIntegrationArgs{})
	for _, field := range []string{"ClientSecret", "GiteaInternalURL"} {
		f, _ := typ.FieldByName(field)
		require.Equal(t, "secret", f.Tag.Get("provider"))
	}
	state := reflect.TypeOf(GiteaIntegrationState{})
	for field, tag := range map[string]string{"GiteaID": "giteaId", "GitProviderID": "gitProviderId", "OrganizationID": "organizationId"} {
		f, _ := state.FieldByName(field)
		require.Equal(t, tag, f.Tag.Get("pulumi"))
		require.Empty(t, f.Tag.Get("provider"))
	}
}

func TestGiteaIntegrationURLValidation(t *testing.T) {
	args := GiteaIntegrationArgs{Name: "gitea", GiteaURL: "https://gitea.example/path", ClientID: "id", ClientSecret: "secret", RedirectURI: "http://example.com/callback"}
	require.NoError(t, validateGiteaIntegrationArgs(args))
	for _, raw := range []string{"https://user:pass@gitea.example", "https://gitea.example/path?x=y", "https://gitea.example/path?", "https://gitea.example/path#x", "https://gitea.example/path#", "ftp://gitea.example", "https:///path"} {
		t.Run(raw, func(t *testing.T) {
			a := args
			a.GiteaURL = raw
			require.Error(t, validateGiteaIntegrationArgs(a))
		})
	}
	args.RedirectURI = "https://user:pass@example.com/callback"
	require.Error(t, validateGiteaIntegrationArgs(args))
	args.RedirectURI = "https://example.com/callback"
	args.GiteaInternalURL = stringPtr("https://gitea.internal/path")
	require.NoError(t, validateGiteaIntegrationArgs(args))
}

func TestGiteaIntegrationDiff(t *testing.T) {
	old := GiteaIntegrationArgs{Name: "old", GiteaURL: "https://gitea.example", ClientID: "id", ClientSecret: "secret", RedirectURI: "https://example.com/callback"}
	in := old
	in.Name, in.GiteaURL, in.ClientID, in.ClientSecret = "new", "https://gitea.example/path", "new-id", "new-secret"
	in.RedirectURI, in.OrganizationName = "https://example.com/new", "org"
	in.GiteaInternalURL = stringPtr("https://internal.example")
	changes, err := (GiteaIntegration{}).Diff(t.Context(), infer.DiffRequest[GiteaIntegrationArgs, GiteaIntegrationState]{Inputs: in, State: GiteaIntegrationState{GiteaIntegrationArgs: old}})
	require.NoError(t, err)
	require.True(t, changes.HasChanges)
	for _, field := range []string{"name", "giteaUrl", "clientId", "clientSecret", "redirectUri", "organizationName", "giteaInternalUrl"} {
		require.Equal(t, p.Update, changes.DetailedDiff[field].Kind)
	}
	unchanged, err := (GiteaIntegration{}).Diff(context.Background(), infer.DiffRequest[GiteaIntegrationArgs, GiteaIntegrationState]{Inputs: old, State: GiteaIntegrationState{GiteaIntegrationArgs: old}})
	require.NoError(t, err)
	require.False(t, unchanged.HasChanges)
	require.Empty(t, unchanged.DetailedDiff)
}
