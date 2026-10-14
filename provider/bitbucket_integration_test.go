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

func TestValidateBitbucketIntegrationArgs(t *testing.T) {
	valid := []BitbucketIntegrationArgs{
		{Name: "integration", APIToken: ptr("token"), BitbucketEmail: ptr("a@example.com")},
		{Name: "integration", AppPassword: ptr("password"), BitbucketUsername: ptr("user")},
	}
	for _, args := range valid {
		if err := validateBitbucketIntegrationArgs(args); err != nil {
			t.Fatalf("valid args rejected: %v", err)
		}
	}
	invalid := []BitbucketIntegrationArgs{
		{Name: "integration"},
		{Name: "integration", APIToken: ptr("token"), AppPassword: ptr("password"), BitbucketEmail: ptr("a@example.com")},
		{Name: "integration", APIToken: ptr(""), BitbucketEmail: ptr("a@example.com")},
		{Name: "integration", APIToken: ptr("token")},
		{Name: "integration", AppPassword: ptr("password"), BitbucketUsername: ptr("")},
		{Name: "integration", AppPassword: ptr("password"), BitbucketUsername: ptr("user"), BitbucketEmail: ptr("bad")},
		{Name: "integration", AppPassword: ptr("password"), BitbucketUsername: ptr("user"), BitbucketEmail: ptr("")},
	}
	for _, args := range invalid {
		if err := validateBitbucketIntegrationArgs(args); err == nil {
			t.Fatalf("invalid args accepted: %#v", args)
		}
	}
}

func TestBitbucketSecretAnnotations(t *testing.T) {
	typ := reflect.TypeOf(BitbucketIntegrationArgs{})
	for _, field := range []string{"APIToken", "AppPassword"} {
		tag, ok := typ.FieldByName(field)
		if !ok || tag.Tag.Get("pulumi") == "" || tag.Tag.Get("provider") != "secret" {
			t.Fatalf("%s secret metadata missing: %v", field, tag)
		}
	}
}

func TestBitbucketCheckValidatesKnownFieldsIndependently(t *testing.T) {
	values := property.NewMap(map[string]property.Value{"name": property.New("x"), "apiToken": property.New(property.Computed), "bitbucketEmail": property.New("bad")})
	resp, err := (BitbucketIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: values})
	require.NoError(t, err)
	require.Contains(t, []string{resp.Failures[0].Property}, "bitbucketEmail")
}

func TestBitbucketCheckKnownModeSurvivesOtherComputedCredential(t *testing.T) {
	values := property.NewMap(map[string]property.Value{"name": property.New("x"), "apiToken": property.New("token"), "appPassword": property.New(property.Computed)})
	resp, err := (BitbucketIntegration{}).Check(t.Context(), infer.CheckRequest{NewInputs: values})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Failures)
	require.Equal(t, "bitbucketEmail", resp.Failures[0].Property)
}

func TestBitbucketDiffOptionalPreservation(t *testing.T) {
	old := BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "n", APIToken: ptr("t"), BitbucketEmail: ptr("a@example.com"), BitbucketUsername: ptr("inactive"), BitbucketWorkspaceName: ptr("w")}}
	got, err := (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: BitbucketIntegrationArgs{Name: "n", APIToken: ptr("t"), BitbucketEmail: ptr("a@example.com")}, State: old})
	require.NoError(t, err)
	require.False(t, got.HasChanges)
	got, err = (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: BitbucketIntegrationArgs{Name: "n", AppPassword: ptr("pw"), BitbucketUsername: ptr("u")}, State: old})
	require.NoError(t, err)
	require.True(t, got.HasChanges)
	require.Equal(t, p.Update, got.DetailedDiff["appPassword"].Kind)
}

func TestBitbucketDiffCoversEveryInputAndIndependentOptionalOmissions(t *testing.T) {
	old := BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{
		Name: "old", APIToken: ptr("api-old"), AppPassword: ptr("app-old"),
		BitbucketEmail: ptr("old@example.invalid"), BitbucketUsername: ptr("old-user"), BitbucketWorkspaceName: ptr("old-workspace"),
	}}
	changed := BitbucketIntegrationArgs{
		Name: "new", APIToken: ptr("api-new"), AppPassword: ptr("app-new"),
		BitbucketEmail: ptr("new@example.invalid"), BitbucketUsername: ptr("new-user"), BitbucketWorkspaceName: ptr("new-workspace"),
	}
	got, err := (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: changed, State: old})
	require.NoError(t, err)
	require.True(t, got.HasChanges)
	for _, key := range []string{"name", "apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"} {
		require.Equal(t, p.Update, got.DetailedDiff[key].Kind, key)
	}
	for _, tc := range []struct {
		key    string
		change func(*BitbucketIntegrationArgs)
	}{
		{"name", func(a *BitbucketIntegrationArgs) { a.Name = "new" }},
		{"apiToken", func(a *BitbucketIntegrationArgs) { a.APIToken = ptr("api-new") }},
		{"appPassword", func(a *BitbucketIntegrationArgs) { a.AppPassword = ptr("app-new") }},
		{"bitbucketEmail", func(a *BitbucketIntegrationArgs) { a.BitbucketEmail = ptr("new@example.invalid") }},
		{"bitbucketUsername", func(a *BitbucketIntegrationArgs) { a.BitbucketUsername = ptr("new-user") }},
		{"bitbucketWorkspaceName", func(a *BitbucketIntegrationArgs) { a.BitbucketWorkspaceName = ptr("new-workspace") }},
	} {
		t.Run("single-change/"+tc.key, func(t *testing.T) {
			inputs := old.BitbucketIntegrationArgs
			tc.change(&inputs)
			diff, err := (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: inputs, State: old})
			require.NoError(t, err)
			require.True(t, diff.HasChanges)
			require.Equal(t, map[string]p.PropertyDiff{tc.key: {Kind: p.Update}}, diff.DetailedDiff)
		})
	}

	for _, key := range []string{"bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"} {
		t.Run("omitted/"+key, func(t *testing.T) {
			inputs := BitbucketIntegrationArgs{Name: "old", APIToken: ptr("api-old"), AppPassword: ptr("app-old"), BitbucketEmail: ptr("old@example.invalid"), BitbucketUsername: ptr("old-user"), BitbucketWorkspaceName: ptr("old-workspace")}
			switch key {
			case "bitbucketEmail":
				inputs.BitbucketEmail = nil
			case "bitbucketUsername":
				inputs.BitbucketUsername = nil
			case "bitbucketWorkspaceName":
				inputs.BitbucketWorkspaceName = nil
			}
			diff, err := (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: inputs, State: old})
			require.NoError(t, err)
			require.False(t, diff.HasChanges, "omitting this inactive optional value preserves it")
			require.Empty(t, diff.DetailedDiff)
		})
	}

	t.Run("credential mode switch", func(t *testing.T) {
		inputs := BitbucketIntegrationArgs{Name: "old", AppPassword: ptr("app-new"), BitbucketUsername: ptr("new-user")}
		diff, err := (BitbucketIntegration{}).Diff(context.Background(), infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]{Inputs: inputs, State: old})
		require.NoError(t, err)
		require.True(t, diff.HasChanges)
		require.Equal(t, p.Update, diff.DetailedDiff["apiToken"].Kind)
		require.Equal(t, p.Update, diff.DetailedDiff["appPassword"].Kind)
		require.Equal(t, p.Update, diff.DetailedDiff["bitbucketUsername"].Kind)
		require.NotContains(t, diff.DetailedDiff, "bitbucketEmail", "omitted email is not a clear")
		require.NotContains(t, diff.DetailedDiff, "bitbucketWorkspaceName", "omitted workspace is not a clear")
	})
}
