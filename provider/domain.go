package dokploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type CertificateType string

const (
	CertificateLetsencrypt CertificateType = "letsencrypt"
	CertificateNone        CertificateType = "none"
	CertificateCustom      CertificateType = "custom"
)

type DomainArgs struct {
	ApplicationID      *string         `pulumi:"applicationId,optional" provider:"replaceOnChanges"`
	ComposeID          *string         `pulumi:"composeId,optional" provider:"replaceOnChanges"`
	ServiceName        *string         `pulumi:"serviceName,optional" provider:"replaceOnChanges"`
	Host               string          `pulumi:"host"`
	Path               *string         `pulumi:"path,optional"`
	InternalPath       *string         `pulumi:"internalPath,optional"`
	Port               *int            `pulumi:"port,optional"`
	HTTPS              bool            `pulumi:"https,optional"`
	CertificateType    CertificateType `pulumi:"certificateType,optional"`
	CustomCertResolver *string         `pulumi:"customCertResolver,optional"`
	StripPath          bool            `pulumi:"stripPath,optional"`
	Enabled            bool            `pulumi:"enabled,optional"`
}

type DomainState struct {
	DomainArgs
	DomainID string `pulumi:"domainId"`
}
type Domain struct{ client clientFactory }

func (r *Domain) Annotate(a infer.Annotator) {
	a.SetToken("index", "Domain")
	a.Describe(&r, "A Dokploy domain routing rule.")
}
func (a *DomainArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.ApplicationID, "The optional application ID target.")
	annotator.Describe(&a.ComposeID, "The optional Compose ID target.")
	annotator.Describe(&a.ServiceName, "The Compose service name.")
	annotator.Describe(&a.Host, "The domain hostname.")
	annotator.Describe(&a.Path, "The public URL path.")
	annotator.Describe(&a.InternalPath, "The internal URL path.")
	annotator.Describe(&a.Port, "The target port.")
	annotator.Describe(&a.HTTPS, "Whether HTTPS is enabled.")
	annotator.Describe(&a.CertificateType, "The certificate resolver type.")
	annotator.Describe(&a.CustomCertResolver, "The custom certificate resolver.")
	annotator.Describe(&a.StripPath, "Whether to strip the public path.")
	annotator.Describe(&a.Enabled, "Whether the domain is enabled.")
	annotator.SetDefault(&a.HTTPS, true)
	annotator.SetDefault(&a.CertificateType, string(CertificateLetsencrypt))
	annotator.SetDefault(&a.Enabled, true)
}

func (a DomainArgs) validate() error {
	hasApp := a.ApplicationID != nil && *a.ApplicationID != ""
	hasCompose := a.ComposeID != nil && *a.ComposeID != ""
	hasService := a.ServiceName != nil && *a.ServiceName != ""
	if hasApp == hasCompose {
		return fmt.Errorf("exactly one of applicationId or composeId must be set")
	}
	if hasCompose && !hasService {
		return fmt.Errorf("serviceName must be set when composeId is set")
	}
	if hasApp && hasService {
		return fmt.Errorf("serviceName must not be set when applicationId is set")
	}
	if a.CertificateType == CertificateCustom && (a.CustomCertResolver == nil || *a.CustomCertResolver == "") {
		return fmt.Errorf("customCertResolver must be set for custom certificate")
	}
	return nil
}

func (r Domain) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[DomainArgs], error) {
	_, explicitHTTPS := req.NewInputs.AsMap()["https"]
	_, explicitEnabled := req.NewInputs.AsMap()["enabled"]
	var explicitHTTPSValue, explicitEnabledValue bool
	if explicitHTTPS {
		explicitHTTPSValue = req.NewInputs.Get("https").AsBool()
	}
	if explicitEnabled {
		explicitEnabledValue = req.NewInputs.Get("enabled").AsBool()
	}
	in, failures, err := infer.DefaultCheck[DomainArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[DomainArgs]{Inputs: in, Failures: failures}, err
	}
	if !explicitHTTPS {
		in.HTTPS = true
	} else {
		in.HTTPS = explicitHTTPSValue
	}
	if in.CertificateType == "" {
		in.CertificateType = CertificateLetsencrypt
	}
	if !explicitEnabled {
		in.Enabled = true
	} else {
		in.Enabled = explicitEnabledValue
	}
	if in.Host == "" {
		failures = append(failures, p.CheckFailure{Property: "host", Reason: "host must not be empty"})
	}
	if in.CertificateType != CertificateLetsencrypt && in.CertificateType != CertificateNone && in.CertificateType != CertificateCustom {
		failures = append(failures, p.CheckFailure{Property: "certificateType", Reason: "certificateType must be one of letsencrypt, none, custom"})
	}
	if err := in.validate(); err != nil {
		failures = append(failures, p.CheckFailure{Property: "target", Reason: err.Error()})
	}
	return infer.CheckResponse[DomainArgs]{Inputs: in, Failures: failures}, nil
}

func (r Domain) Diff(_ context.Context, req infer.DiffRequest[DomainArgs, DomainState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.DomainArgs
	d := map[string]p.PropertyDiff{}
	if !sameOptionalString(in.ApplicationID, old.ApplicationID) {
		d["applicationId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !sameOptionalString(in.ComposeID, old.ComposeID) {
		d["composeId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"serviceName", !sameOptionalString(in.ServiceName, old.ServiceName)}, {"host", in.Host != old.Host}, {"path", !sameOptionalString(in.Path, old.Path)}, {"internalPath", !sameOptionalString(in.InternalPath, old.InternalPath)}, {"port", !sameOptionalInt(in.Port, old.Port)}, {"https", in.HTTPS != old.HTTPS}, {"certificateType", in.CertificateType != old.CertificateType}, {"customCertResolver", !sameOptionalString(in.CustomCertResolver, old.CustomCertResolver)}, {"stripPath", in.StripPath != old.StripPath},
		{"enabled", in.Enabled != old.Enabled},
	} {
		if field.changed {
			kind := p.Update
			if field.name == "serviceName" {
				kind = p.UpdateReplace
			}
			d[field.name] = p.PropertyDiff{Kind: kind}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func nullableString(v *string) nullable.Nullable[string] {
	if v == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
}
func nullablePort(v *int) nullable.Nullable[float32] {
	if v == nil {
		return nullable.NewNullNullable[float32]()
	}
	return nullable.NewNullableWithValue(float32(*v))
}
func domainCreateBody(a DomainArgs) generated.DomainCreateJSONRequestBody {
	b := generated.DomainCreateJSONRequestBody{Host: a.Host, Https: &a.HTTPS, StripPath: &a.StripPath}
	certificate := a.CertificateType
	if certificate == "" {
		certificate = CertificateLetsencrypt
	}
	b.CertificateType = ptr(generated.DomainCreateJSONBodyCertificateType(certificate))
	if a.Path != nil {
		b.Path = nullable.NewNullableWithValue(*a.Path)
	}
	if a.InternalPath != nil {
		b.InternalPath = nullable.NewNullableWithValue(*a.InternalPath)
	}
	if a.Port != nil {
		b.Port = nullable.NewNullableWithValue(float32(*a.Port))
	}
	if a.ServiceName != nil {
		b.ServiceName = nullable.NewNullableWithValue(*a.ServiceName)
	}
	if a.CustomCertResolver != nil {
		b.CustomCertResolver = nullable.NewNullableWithValue(*a.CustomCertResolver)
	}
	if a.ApplicationID != nil {
		b.ApplicationId = nullable.NewNullableWithValue(*a.ApplicationID)
		b.DomainType = nullable.NewNullableWithValue(generated.DomainCreateJSONBodyDomainTypeApplication)
	}
	if a.ComposeID != nil {
		b.ComposeId = nullable.NewNullableWithValue(*a.ComposeID)
		b.DomainType = nullable.NewNullableWithValue(generated.DomainCreateJSONBodyDomainTypeCompose)
	}
	return b
}
func domainUpdateBody(id string, a DomainArgs) generated.DomainUpdateJSONRequestBody {
	b := generated.DomainUpdateJSONRequestBody{DomainId: id, Host: a.Host, Https: &a.HTTPS, StripPath: &a.StripPath, Path: nullableString(a.Path), InternalPath: nullableString(a.InternalPath), Port: nullablePort(a.Port), ServiceName: nullableString(a.ServiceName), CustomCertResolver: nullableString(a.CustomCertResolver)}
	certificate := a.CertificateType
	if certificate == "" {
		certificate = CertificateLetsencrypt
	}
	b.CertificateType = ptr(generated.DomainUpdateJSONBodyCertificateType(certificate))
	if a.ApplicationID != nil {
		b.DomainType = nullable.NewNullableWithValue(generated.DomainUpdateJSONBodyDomainTypeApplication)
	}
	if a.ComposeID != nil {
		b.DomainType = nullable.NewNullableWithValue(generated.DomainUpdateJSONBodyDomainTypeCompose)
	}
	return b
}

func updateDomain(ctx context.Context, api *client.Client, id string, args DomainArgs, enabledOnly bool) error {
	var payload map[string]any
	if enabledOnly {
		payload = map[string]any{"domainId": id, "enabled": args.Enabled}
	} else {
		encoded, err := json.Marshal(domainUpdateBody(id, args))
		if err != nil {
			return fmt.Errorf("domain.update request encoding failed: %w", err)
		}
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return fmt.Errorf("domain.update request encoding failed: %w", err)
		}
		payload["enabled"] = args.Enabled
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("domain.update request encoding failed: %w", err)
	}
	response, err := api.DomainUpdateWithBody(ctx, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("domain.update returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (r Domain) Create(ctx context.Context, req infer.CreateRequest[DomainArgs]) (infer.CreateResponse[DomainState], error) {
	state := DomainState{DomainArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[DomainState]{Output: state}, nil
	}
	api := r.client(ctx)
	resp, err := api.DomainCreateWithResponse(ctx, domainCreateBody(req.Inputs))
	if err != nil {
		return infer.CreateResponse[DomainState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.DomainId == nil {
		return infer.CreateResponse[DomainState]{}, fmt.Errorf("domain.create returned incomplete domain")
	}
	state.DomainID = *resp.JSON200.DomainId
	if !req.Inputs.Enabled {
		if err = updateDomain(ctx, api, state.DomainID, req.Inputs, true); err != nil {
			return infer.CreateResponse[DomainState]{ID: state.DomainID, Output: state}, err
		}
	}
	return infer.CreateResponse[DomainState]{ID: state.DomainID, Output: state}, nil
}

func (r Domain) Read(ctx context.Context, req infer.ReadRequest[DomainArgs, DomainState]) (infer.ReadResponse[DomainArgs, DomainState], error) {
	resp, err := r.client(ctx).DomainOneWithResponse(ctx, &generated.DomainOneParams{DomainId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[DomainArgs, DomainState]{ID: ""}, nil
		}
		return infer.ReadResponse[DomainArgs, DomainState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.DomainId == nil {
		return infer.ReadResponse[DomainArgs, DomainState]{}, fmt.Errorf("domain.one returned incomplete domain")
	}
	d := resp.JSON200
	a := DomainArgs{ApplicationID: d.ApplicationId, ComposeID: d.ComposeId, ServiceName: d.ServiceName, Path: d.Path, InternalPath: d.InternalPath, Port: d.Port, CustomCertResolver: d.CustomCertResolver}
	a.Host = value(d.Host)
	a.HTTPS = valueBool(d.Https)
	a.CertificateType = CertificateType(value(d.CertificateType))
	a.StripPath = valueBool(d.StripPath)
	a.Enabled = req.State.Enabled
	if d.Enabled != nil {
		a.Enabled = *d.Enabled
	}
	return infer.ReadResponse[DomainArgs, DomainState]{ID: *d.DomainId, Inputs: a, State: DomainState{DomainArgs: a, DomainID: *d.DomainId}}, nil
}
func valueBool(v *bool) bool { return v != nil && *v }
func (r Domain) Update(ctx context.Context, req infer.UpdateRequest[DomainArgs, DomainState]) (infer.UpdateResponse[DomainState], error) {
	st := DomainState{DomainArgs: req.Inputs, DomainID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[DomainState]{Output: st}, nil
	}
	if err := updateDomain(ctx, r.client(ctx), req.ID, req.Inputs, false); err != nil {
		return infer.UpdateResponse[DomainState]{Output: st}, err
	}
	return infer.UpdateResponse[DomainState]{Output: st}, nil
}
func (r Domain) Delete(ctx context.Context, req infer.DeleteRequest[DomainState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).DomainDeleteWithResponse(ctx, generated.DomainDeleteJSONRequestBody{DomainId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r Domain) WireDependencies(f infer.FieldSelector, args *DomainArgs, state *DomainState) {
	deps := []infer.InputField{f.InputField(&args.ApplicationID), f.InputField(&args.ComposeID), f.InputField(&args.ServiceName), f.InputField(&args.Host), f.InputField(&args.Path), f.InputField(&args.InternalPath), f.InputField(&args.Port), f.InputField(&args.HTTPS), f.InputField(&args.CertificateType), f.InputField(&args.CustomCertResolver), f.InputField(&args.StripPath), f.InputField(&args.Enabled)}
	f.OutputField(&state.DomainID).DependsOn(deps...)
}
