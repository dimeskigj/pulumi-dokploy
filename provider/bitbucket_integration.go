package dokploy

import (
	"context"
	"fmt"
	"net/mail"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type BitbucketIntegrationArgs struct {
	Name                   string  `pulumi:"name"`
	APIToken               *string `pulumi:"apiToken,optional" provider:"secret"`
	AppPassword            *string `pulumi:"appPassword,optional" provider:"secret"`
	BitbucketEmail         *string `pulumi:"bitbucketEmail,optional"`
	BitbucketUsername      *string `pulumi:"bitbucketUsername,optional"`
	BitbucketWorkspaceName *string `pulumi:"bitbucketWorkspaceName,optional"`
}

type BitbucketIntegrationState struct {
	BitbucketIntegrationArgs
	BitbucketID    string `pulumi:"bitbucketId"`
	GitProviderID  string `pulumi:"gitProviderId"`
	OrganizationID string `pulumi:"organizationId"`
}

func (a *BitbucketIntegrationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The Dokploy integration display name.")
	n.Describe(&a.APIToken, "A secret Bitbucket API token; select exactly one credential mode.")
	n.Describe(&a.AppPassword, "A secret Bitbucket app password; select exactly one credential mode.")
	n.Describe(&a.BitbucketEmail, "The email required for API-token mode.")
	n.Describe(&a.BitbucketUsername, "The username required for app-password mode.")
	n.Describe(&a.BitbucketWorkspaceName, "An optional Bitbucket workspace; omission preserves the existing value.")
}

func (s *BitbucketIntegrationState) Annotate(n infer.Annotator) {
	n.Describe(&s.BitbucketID, "The stable Bitbucket relation ID used as the Pulumi resource identity.")
	n.Describe(&s.GitProviderID, "The associated Dokploy Git-provider ID.")
	n.Describe(&s.OrganizationID, "The owning Dokploy organization ID.")
}

type BitbucketIntegration struct{ client clientFactory }

func (r *BitbucketIntegration) Annotate(a infer.Annotator) {
	a.SetToken("index", "BitbucketIntegration")
	a.Describe(&r, "Manages a Dokploy Bitbucket integration using an API token or legacy app password.")
}

func (r BitbucketIntegration) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[BitbucketIntegrationArgs], error) {
	in, failures, err := infer.DefaultCheck[BitbucketIntegrationArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[BitbucketIntegrationArgs]{Inputs: in, Failures: failures}, err
	}
	add := func(name, reason string) { failures = append(failures, p.CheckFailure{Property: name, Reason: reason}) }
	for _, field := range []struct{ name, value string }{{"name", in.Name}} {
		v := req.NewInputs.Get(field.name)
		if !v.HasComputed() && (v.IsNull() || field.value == "") {
			add(field.name, field.name+" must not be empty")
		}
	}
	for _, name := range []string{"apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"} {
		v, ok := req.NewInputs.GetOk(name)
		if !ok || v.IsNull() || v.HasComputed() {
			continue
		}
		if !v.IsString() {
			continue
		}
		value := v.AsString()
		if name == "bitbucketEmail" && !validBitbucketEmail(value) {
			add(name, "bitbucketEmail must be a valid email address")
		}
		if (name == "apiToken" || name == "appPassword") && value == "" {
			continue
		}
	}
	apiKnown := req.NewInputs.Get("apiToken")
	appKnown := req.NewInputs.Get("appPassword")
	api := in.APIToken != nil && *in.APIToken != ""
	app := in.AppPassword != nil && *in.AppPassword != ""
	if !apiKnown.HasComputed() && !appKnown.HasComputed() && api == app {
		add("apiToken", "exactly one of apiToken and appPassword must be nonempty")
	}
	if api && !req.NewInputs.Get("bitbucketEmail").HasComputed() && (in.BitbucketEmail == nil || *in.BitbucketEmail == "") {
		add("bitbucketEmail", "bitbucketEmail is required for apiToken mode")
	}
	if app && !req.NewInputs.Get("bitbucketUsername").HasComputed() && (in.BitbucketUsername == nil || *in.BitbucketUsername == "") {
		add("bitbucketUsername", "bitbucketUsername is required for appPassword mode")
	}
	return infer.CheckResponse[BitbucketIntegrationArgs]{Inputs: in, Failures: failures}, nil
}

func validateBitbucketIntegrationArgs(a BitbucketIntegrationArgs) error {
	if a.Name == "" {
		return fmt.Errorf("name must not be empty")
	}
	api := a.APIToken != nil && *a.APIToken != ""
	app := a.AppPassword != nil && *a.AppPassword != ""
	if api == app {
		return fmt.Errorf("exactly one of apiToken and appPassword must be nonempty")
	}
	if a.BitbucketEmail != nil && !validBitbucketEmail(*a.BitbucketEmail) {
		return fmt.Errorf("bitbucketEmail must be a valid email address")
	}
	if api && (a.BitbucketEmail == nil || *a.BitbucketEmail == "") {
		return fmt.Errorf("bitbucketEmail is required for apiToken mode")
	}
	if app && (a.BitbucketUsername == nil || *a.BitbucketUsername == "") {
		return fmt.Errorf("bitbucketUsername is required for appPassword mode")
	}
	return nil
}

func validBitbucketEmail(raw string) bool {
	a, err := mail.ParseAddress(raw)
	return err == nil && a.Address == raw && a.Name == "" && a.Address != ""
}

func (r BitbucketIntegration) Diff(_ context.Context, req infer.DiffRequest[BitbucketIntegrationArgs, BitbucketIntegrationState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.BitbucketIntegrationArgs
	d := map[string]p.PropertyDiff{}
	fields := []struct {
		name    string
		changed bool
	}{
		{"name", in.Name != old.Name}, {"apiToken", !sameOptionalString(in.APIToken, old.APIToken)}, {"appPassword", !sameOptionalString(in.AppPassword, old.AppPassword)},
		{"bitbucketEmail", in.BitbucketEmail != nil && !sameOptionalString(in.BitbucketEmail, old.BitbucketEmail)},
		{"bitbucketUsername", in.BitbucketUsername != nil && !sameOptionalString(in.BitbucketUsername, old.BitbucketUsername)},
		{"bitbucketWorkspaceName", in.BitbucketWorkspaceName != nil && !sameOptionalString(in.BitbucketWorkspaceName, old.BitbucketWorkspaceName)},
	}
	for _, f := range fields {
		if f.changed {
			d[f.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func bitbucketIntegrationPreviewState(args BitbucketIntegrationArgs, id string, prior BitbucketIntegrationState) BitbucketIntegrationState {
	old := prior.BitbucketIntegrationArgs
	if args.BitbucketEmail == nil {
		args.BitbucketEmail = old.BitbucketEmail
	}
	if args.BitbucketUsername == nil {
		args.BitbucketUsername = old.BitbucketUsername
	}
	if args.BitbucketWorkspaceName == nil {
		args.BitbucketWorkspaceName = old.BitbucketWorkspaceName
	}
	prior.BitbucketIntegrationArgs = args
	if id != "" {
		prior.BitbucketID = id
	}
	return prior
}

func (r BitbucketIntegration) WireDependencies(f infer.FieldSelector, args *BitbucketIntegrationArgs, state *BitbucketIntegrationState) {
	f.OutputField(&state.Name).DependsOn(f.InputField(&args.Name).Computed())
	f.OutputField(&state.APIToken).DependsOn(f.InputField(&args.APIToken).Computed(), f.InputField(&args.APIToken).Secret())
	f.OutputField(&state.AppPassword).DependsOn(f.InputField(&args.AppPassword).Computed(), f.InputField(&args.AppPassword).Secret())
	f.OutputField(&state.BitbucketEmail).DependsOn(f.InputField(&args.BitbucketEmail).Computed())
	f.OutputField(&state.BitbucketUsername).DependsOn(f.InputField(&args.BitbucketUsername).Computed())
	f.OutputField(&state.BitbucketWorkspaceName).DependsOn(f.InputField(&args.BitbucketWorkspaceName).Computed())
	f.OutputField(&state.BitbucketID).DependsOn()
	f.OutputField(&state.GitProviderID).DependsOn()
	f.OutputField(&state.OrganizationID).DependsOn()
	f.OutputField(&state.BitbucketID).NeverSecret()
	f.OutputField(&state.GitProviderID).NeverSecret()
	f.OutputField(&state.OrganizationID).NeverSecret()
}
