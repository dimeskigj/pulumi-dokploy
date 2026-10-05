package dokploy

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GitLabIntegrationArgs struct {
	Name              string  `pulumi:"name"`
	ApplicationID     string  `pulumi:"applicationId"`
	ApplicationSecret string  `pulumi:"applicationSecret" provider:"secret"`
	RedirectURI       string  `pulumi:"redirectUri"`
	GitlabURL         string  `pulumi:"gitlabUrl,optional"`
	GroupName         string  `pulumi:"groupName,optional"`
	GitlabInternalURL *string `pulumi:"gitlabInternalUrl,optional" provider:"secret"`
}

const gitLabHTTPSScheme = "https"

type GitLabIntegrationState struct {
	GitLabIntegrationArgs
	GitLabID       string `pulumi:"gitlabId"`
	GitProviderID  string `pulumi:"gitProviderId"`
	OrganizationID string `pulumi:"organizationId"`
	IsConfigured   bool   `pulumi:"isConfigured"`
}

func (s *GitLabIntegrationState) Annotate(a infer.Annotator) {
	a.Describe(&s.GitLabID, "The stable GitLab integration ID used as the Pulumi resource identity.")
	a.Describe(&s.GitProviderID, "The Dokploy Git provider ID associated with this integration.")
	a.Describe(&s.OrganizationID, "The Dokploy organization ID associated with this integration.")
	a.Describe(&s.IsConfigured, "Whether access and refresh tokens are present; this does not indicate connectivity or authorization validity.")
}

type GitLabIntegration struct{ client clientFactory }

func (r *GitLabIntegration) Annotate(a infer.Annotator) {
	a.SetToken("index", "GitLabIntegration")
	a.Describe(&r, "A Dokploy GitLab OAuth integration. Configure OAuth manually in GitLab and Dokploy; readiness does not verify connectivity or authorization.")
}

func (a *GitLabIntegrationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The integration name.")
	n.Describe(&a.ApplicationID, "The GitLab OAuth application ID.")
	n.Describe(&a.ApplicationSecret, "The GitLab OAuth application secret.")
	n.Describe(&a.RedirectURI, "The OAuth callback URI configured for the application.")
	n.Describe(&a.GitlabURL, "The public GitLab base URL; defaults to https://gitlab.com.")
	n.Describe(&a.GroupName, "The optional GitLab group name; defaults to an empty string.")
	n.SetDefault(&a.GitlabURL, "https://gitlab.com")
	n.SetDefault(&a.GroupName, "")
	n.Describe(&a.GitlabInternalURL, "An optional internal GitLab URL override, which may include basic-auth credentials.")
}

func (r GitLabIntegration) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[GitLabIntegrationArgs], error) {
	in, failures, err := infer.DefaultCheck[GitLabIntegrationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[GitLabIntegrationArgs]{Inputs: in, Failures: failures}, err
	}
	if _, ok := req.NewInputs.GetOk("gitlabUrl"); !ok {
		in.GitlabURL = "https://gitlab.com"
	}
	if _, ok := req.NewInputs.GetOk("groupName"); !ok {
		in.GroupName = ""
	}
	add := func(name, reason string) { failures = append(failures, p.CheckFailure{Property: name, Reason: reason}) }
	for _, field := range []struct{ name, value string }{
		{"name", in.Name}, {"applicationId", in.ApplicationID}, {"applicationSecret", in.ApplicationSecret}, {"redirectUri", in.RedirectURI},
	} {
		v := req.NewInputs.Get(field.name)
		if !v.HasComputed() && field.value == "" {
			add(field.name, field.name+" must not be empty")
		}
	}
	for _, name := range []string{"gitlabUrl", "redirectUri"} {
		v, specified := req.NewInputs.GetOk(name)
		if v.HasComputed() {
			continue
		}
		value := in.GitlabURL
		if name == "redirectUri" {
			value = in.RedirectURI
		}
		if (specified && (v.IsNull() || (v.IsString() && v.AsString() == ""))) || !validWebURL(value, false) {
			add(name, name+" must be an absolute HTTP(S) URL with a host and no userinfo")
		}
	}
	if v, specified := req.NewInputs.GetOk("gitlabInternalUrl"); specified && !v.IsNull() && !v.HasComputed() {
		if in.GitlabInternalURL == nil || !validWebURL(*in.GitlabInternalURL, true) {
			add("gitlabInternalUrl", "gitlabInternalUrl must be an absolute HTTP(S) URL with a host")
		}
	}
	return infer.CheckResponse[GitLabIntegrationArgs]{Inputs: in, Failures: failures}, nil
}

func validateGitLabIntegrationArgs(a GitLabIntegrationArgs) error {
	for _, field := range []struct{ name, value string }{
		{"name", a.Name}, {"applicationId", a.ApplicationID}, {"applicationSecret", a.ApplicationSecret}, {"redirectUri", a.RedirectURI}, {"gitlabUrl", a.GitlabURL},
	} {
		if field.value == "" {
			return fmt.Errorf("%s must not be empty", field.name)
		}
	}
	if !validWebURL(a.GitlabURL, false) {
		return fmt.Errorf("gitlabUrl must be an absolute HTTP(S) URL with a host and no userinfo")
	}
	if !validWebURL(a.RedirectURI, false) {
		return fmt.Errorf("redirectUri must be an absolute HTTP(S) URL with a host and no userinfo")
	}
	if a.GitlabInternalURL != nil && (*a.GitlabInternalURL == "" || !validWebURL(*a.GitlabInternalURL, true)) {
		return fmt.Errorf("gitlabInternalUrl must be an absolute HTTP(S) URL with a host")
	}
	return nil
}

func validWebURL(raw string, allowUserinfo bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != gitLabHTTPSScheme) || strings.TrimSpace(raw) != raw {
		return false
	}
	return allowUserinfo || u.User == nil
}

func (r GitLabIntegration) Diff(_ context.Context, req infer.DiffRequest[GitLabIntegrationArgs, GitLabIntegrationState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.GitLabIntegrationArgs
	d := map[string]p.PropertyDiff{}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"name", in.Name != old.Name}, {"applicationId", in.ApplicationID != old.ApplicationID},
		{"applicationSecret", in.ApplicationSecret != old.ApplicationSecret}, {"redirectUri", in.RedirectURI != old.RedirectURI},
		{"gitlabUrl", in.GitlabURL != old.GitlabURL}, {"groupName", in.GroupName != old.GroupName},
		{"gitlabInternalUrl", !sameOptionalString(in.GitlabInternalURL, old.GitlabInternalURL)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r GitLabIntegration) WireDependencies(f infer.FieldSelector, args *GitLabIntegrationArgs, state *GitLabIntegrationState) {
	publicInputs := []infer.InputField{
		f.InputField(&args.Name), f.InputField(&args.ApplicationID),
		f.InputField(&args.RedirectURI), f.InputField(&args.GitlabURL), f.InputField(&args.GroupName),
	}
	// The identity is generated on create, but in-place configuration changes
	// cannot change any of these IDs during an update preview.
	f.OutputField(&state.GitLabID).DependsOn()
	f.OutputField(&state.GitProviderID).DependsOn()
	f.OutputField(&state.OrganizationID).DependsOn()
	f.OutputField(&state.GitLabID).NeverSecret()
	f.OutputField(&state.GitProviderID).NeverSecret()
	f.OutputField(&state.OrganizationID).NeverSecret()
	readinessInputs := append(publicInputs, f.InputField(&args.ApplicationSecret).Computed(), f.InputField(&args.GitlabInternalURL).Computed())
	f.OutputField(&state.IsConfigured).DependsOn(readinessInputs...)
	f.OutputField(&state.IsConfigured).NeverSecret()
	f.OutputField(&state.ApplicationSecret).DependsOn(f.InputField(&args.ApplicationSecret).Secret())
	f.OutputField(&state.GitlabInternalURL).DependsOn(f.InputField(&args.GitlabInternalURL).Secret())
}
