package dokploy

import "testing"

func TestBitbucketPreviewState(t *testing.T) {
	prior := BitbucketIntegrationState{BitbucketIntegrationArgs: BitbucketIntegrationArgs{Name: "old", BitbucketUsername: ptr("legacy"), BitbucketWorkspaceName: ptr("workspace")}, BitbucketID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	got := bitbucketIntegrationPreviewState(BitbucketIntegrationArgs{Name: "new", APIToken: ptr("secret")}, "", prior)
	if got.BitbucketUsername == nil || *got.BitbucketUsername != "legacy" || got.BitbucketWorkspaceName == nil || *got.BitbucketWorkspaceName != "workspace" {
		t.Fatalf("omitted optional values not retained: %#v", got)
	}
	if got.BitbucketID != "relation" || got.GitProviderID != "parent" || got.OrganizationID != "org" {
		t.Fatalf("IDs not retained: %#v", got)
	}
	if got.APIToken == nil || *got.APIToken != "secret" || got.AppPassword != nil {
		t.Fatalf("credentials inferred: %#v", got.BitbucketIntegrationArgs)
	}
	got = bitbucketIntegrationPreviewState(BitbucketIntegrationArgs{Name: "new", BitbucketWorkspaceName: ptr("")}, "relation-2", prior)
	if got.BitbucketWorkspaceName == nil || *got.BitbucketWorkspaceName != "" || got.BitbucketID != "relation-2" {
		t.Fatalf("explicit empty/relation not applied: %#v", got)
	}
}
