package dokploy

import (
	"context"
	"fmt"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const githubIntegrationCreateRejected = "GitHubIntegration cannot create an integration; complete GitHub App setup and authorization in Dokploy, then import the GitHub relation ID"

type GitHubIntegrationArgs struct {
	Name          string `pulumi:"name"`
	GitHubAppName string `pulumi:"githubAppName"`
}

type GitHubIntegrationState struct {
	GitHubIntegrationArgs
	GitHubID       string `pulumi:"githubId"`
	GitProviderID  string `pulumi:"gitProviderId"`
	OrganizationID string `pulumi:"organizationId"`
}

func (a *GitHubIntegrationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The required Dokploy integration display name.")
	n.Describe(&a.GitHubAppName, "The GitHub App name stored by Dokploy; this does not configure the GitHub-side App.")
}
func (s *GitHubIntegrationState) Annotate(n infer.Annotator) {
	n.Describe(&s.GitHubID, "The stable Dokploy GitHub relation ID used as the Pulumi resource identity.")
	n.Describe(&s.GitProviderID, "The stable Dokploy Git-provider parent ID.")
	n.Describe(&s.OrganizationID, "The owning Dokploy organization ID.")
}

type GitHubIntegration struct{ client clientFactory }

func (r *GitHubIntegration) Annotate(a infer.Annotator) {
	a.SetToken("index", "GitHubIntegration")
	a.Describe(&r, "Manages an existing Dokploy GitHub integration after setup and authorization in Dokploy; import-only.")
}
func (r GitHubIntegration) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[GitHubIntegrationArgs], error) {
	in, failures, err := infer.DefaultCheck[GitHubIntegrationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[GitHubIntegrationArgs]{Inputs: in, Failures: failures}, err
	}
	for _, field := range []struct{ name, value string }{{"name", in.Name}, {"githubAppName", in.GitHubAppName}} {
		v := req.NewInputs.Get(field.name)
		if !v.HasComputed() && (v.IsNull() || field.value == "") {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: field.name + " must not be empty"})
		}
	}
	return infer.CheckResponse[GitHubIntegrationArgs]{Inputs: in, Failures: failures}, nil
}
func validateGitHubIntegrationArgs(a GitHubIntegrationArgs) error {
	if a.Name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if a.GitHubAppName == "" {
		return fmt.Errorf("githubAppName must not be empty")
	}
	return nil
}
func (r GitHubIntegration) Diff(_ context.Context, req infer.DiffRequest[GitHubIntegrationArgs, GitHubIntegrationState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.GitHubIntegrationArgs
	d := map[string]p.PropertyDiff{}
	if in.Name != old.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if in.GitHubAppName != old.GitHubAppName {
		d["githubAppName"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}
func (r GitHubIntegration) Create(_ context.Context, req infer.CreateRequest[GitHubIntegrationArgs]) (infer.CreateResponse[GitHubIntegrationState], error) {
	if req.DryRun {
		return infer.CreateResponse[GitHubIntegrationState]{Output: GitHubIntegrationState{GitHubIntegrationArgs: req.Inputs}}, nil
	}
	return infer.CreateResponse[GitHubIntegrationState]{}, fmt.Errorf("%s", githubIntegrationCreateRejected)
}
func gitHubIntegrationPreviewState(inputs GitHubIntegrationArgs, relationID string, prior GitHubIntegrationState) GitHubIntegrationState {
	prior.GitHubIntegrationArgs = inputs
	if relationID != "" {
		prior.GitHubID = relationID
	}
	return prior
}
func (r GitHubIntegration) WireDependencies(f infer.FieldSelector, args *GitHubIntegrationArgs, state *GitHubIntegrationState) {
	deps := []infer.InputField{f.InputField(&args.Name), f.InputField(&args.GitHubAppName)}
	f.OutputField(&state.GitHubID).DependsOn(deps...)
	f.OutputField(&state.GitProviderID).DependsOn(deps...)
	f.OutputField(&state.OrganizationID).DependsOn(deps...)
	f.OutputField(&state.GitHubID).NeverSecret()
	f.OutputField(&state.GitProviderID).NeverSecret()
	f.OutputField(&state.OrganizationID).NeverSecret()
}
