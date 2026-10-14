package dokploy

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GiteaIntegrationArgs struct {
	Name             string  `pulumi:"name"`
	GiteaURL         string  `pulumi:"giteaUrl"`
	ClientID         string  `pulumi:"clientId"`
	ClientSecret     string  `pulumi:"clientSecret" provider:"secret"`
	RedirectURI      string  `pulumi:"redirectUri"`
	GiteaInternalURL *string `pulumi:"giteaInternalUrl,optional" provider:"secret"`
	OrganizationName string  `pulumi:"organizationName,optional"`
}

type GiteaIntegrationState struct {
	GiteaIntegrationArgs
	GiteaID        string `pulumi:"giteaId"`
	GitProviderID  string `pulumi:"gitProviderId"`
	OrganizationID string `pulumi:"organizationId"`
}

func (s *GiteaIntegrationState) Annotate(a infer.Annotator) {
	a.Describe(&s.GiteaID, "The stable Gitea integration ID used as the Pulumi resource identity.")
	a.Describe(&s.GitProviderID, "The Dokploy Git provider ID associated with this integration.")
	a.Describe(&s.OrganizationID, "The Dokploy organization ID associated with this integration.")
}

type GiteaIntegration struct{ client clientFactory }

const giteaRedirectURIField = "redirectUri"

func (r *GiteaIntegration) Annotate(a infer.Annotator) {
	a.SetToken("index", "GiteaIntegration")
	a.Describe(&r, "A Dokploy Gitea OAuth integration configured manually by the operator.")
}

func (a *GiteaIntegrationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The integration name.")
	n.Describe(&a.GiteaURL, "The absolute HTTP(S) Gitea base URL; paths are allowed.")
	n.Describe(&a.ClientID, "The Gitea OAuth client ID.")
	n.Describe(&a.ClientSecret, "The secret Gitea OAuth client secret.")
	n.Describe(&a.RedirectURI, "The absolute HTTP(S) OAuth callback URI.")
	n.Describe(&a.GiteaInternalURL, "An optional internal Gitea base URL override.")
	n.Describe(&a.OrganizationName, "The optional Gitea organization name; defaults to an empty string.")
	n.SetDefault(&a.OrganizationName, "")
}

func (r GiteaIntegration) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[GiteaIntegrationArgs], error) {
	in, failures, err := infer.DefaultCheck[GiteaIntegrationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[GiteaIntegrationArgs]{Inputs: in, Failures: failures}, err
	}
	if _, ok := req.NewInputs.GetOk("organizationName"); !ok {
		in.OrganizationName = ""
	}
	add := func(name, reason string) { failures = append(failures, p.CheckFailure{Property: name, Reason: reason}) }
	for _, field := range []struct{ name, value string }{{"name", in.Name}, {"clientId", in.ClientID}, {"clientSecret", in.ClientSecret}, {giteaRedirectURIField, in.RedirectURI}} {
		if v := req.NewInputs.Get(field.name); !v.HasComputed() && field.value == "" {
			add(field.name, field.name+" must not be empty")
		}
	}
	for _, name := range []string{"giteaUrl", giteaRedirectURIField} {
		v, specified := req.NewInputs.GetOk(name)
		if v.HasComputed() {
			continue
		}
		if name == giteaRedirectURIField && in.RedirectURI == "" {
			continue // The required-field check already reports the empty value.
		}
		value := in.GiteaURL
		valid := validateGiteaIntegrationBaseURL(value)
		if name == giteaRedirectURIField {
			valid = validWebURL(in.RedirectURI, false)
		}
		if (specified && (v.IsNull() || (v.IsString() && v.AsString() == ""))) || !valid {
			add(name, name+" must be an absolute HTTP(S) URL with a host and no userinfo, query, or fragment")
		}
	}
	if v, specified := req.NewInputs.GetOk("giteaInternalUrl"); specified && !v.IsNull() && !v.HasComputed() {
		if in.GiteaInternalURL == nil || !validateGiteaIntegrationBaseURL(*in.GiteaInternalURL) {
			add("giteaInternalUrl", "giteaInternalUrl must be an absolute HTTP(S) URL with a host and no userinfo, query, or fragment")
		}
	}
	return infer.CheckResponse[GiteaIntegrationArgs]{Inputs: in, Failures: failures}, nil
}

func validateGiteaIntegrationArgs(a GiteaIntegrationArgs) error {
	for _, f := range []struct{ name, value string }{{"name", a.Name}, {"giteaUrl", a.GiteaURL}, {"clientId", a.ClientID}, {"clientSecret", a.ClientSecret}, {"redirectUri", a.RedirectURI}} {
		if f.value == "" {
			return fmt.Errorf("%s must not be empty", f.name)
		}
	}
	if !validateGiteaIntegrationBaseURL(a.GiteaURL) {
		return fmt.Errorf("giteaUrl must be an absolute HTTP(S) URL with a host and no userinfo, query, or fragment")
	}
	if !validWebURL(a.RedirectURI, false) {
		return fmt.Errorf("redirectUri must be an absolute HTTP(S) URL with a host and no userinfo")
	}
	if a.GiteaInternalURL != nil && (*a.GiteaInternalURL == "" || !validateGiteaIntegrationBaseURL(*a.GiteaInternalURL)) {
		return fmt.Errorf("giteaInternalUrl must be an absolute HTTP(S) URL with a host and no userinfo, query, or fragment")
	}
	return nil
}

func validateGiteaIntegrationBaseURL(raw string) bool {
	if !validWebURL(raw, false) || strings.ContainsAny(raw, "?#") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && !u.ForceQuery && u.RawQuery == "" && u.Fragment == "" && u.User == nil
}

func (r GiteaIntegration) Diff(_ context.Context, req infer.DiffRequest[GiteaIntegrationArgs, GiteaIntegrationState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.GiteaIntegrationArgs
	d := map[string]p.PropertyDiff{}
	for _, f := range []struct {
		name    string
		changed bool
	}{
		{"name", in.Name != old.Name}, {"giteaUrl", in.GiteaURL != old.GiteaURL}, {"clientId", in.ClientID != old.ClientID},
		{"clientSecret", in.ClientSecret != old.ClientSecret}, {"redirectUri", in.RedirectURI != old.RedirectURI},
		{"giteaInternalUrl", !sameOptionalString(in.GiteaInternalURL, old.GiteaInternalURL)}, {"organizationName", in.OrganizationName != old.OrganizationName},
	} {
		if f.changed {
			d[f.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r GiteaIntegration) WireDependencies(f infer.FieldSelector, args *GiteaIntegrationArgs, state *GiteaIntegrationState) {
	f.OutputField(&state.GiteaID).DependsOn()
	f.OutputField(&state.GiteaID).NeverSecret()
	f.OutputField(&state.GitProviderID).DependsOn()
	f.OutputField(&state.GitProviderID).NeverSecret()
	f.OutputField(&state.OrganizationID).DependsOn()
	f.OutputField(&state.OrganizationID).NeverSecret()
	f.OutputField(&state.ClientSecret).DependsOn(f.InputField(&args.ClientSecret).Secret().Computed())
	f.OutputField(&state.GiteaInternalURL).DependsOn(f.InputField(&args.GiteaInternalURL).Secret().Computed())
}
