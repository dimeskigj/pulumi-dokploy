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
