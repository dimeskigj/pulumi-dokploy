package dokploy

import (
	"context"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type SSHKeyArgs struct {
	Name        string  `pulumi:"name"`
	Description *string `pulumi:"description,optional"`
	PrivateKey  string  `pulumi:"privateKey" provider:"secret,replaceOnChanges"`
	PublicKey   string  `pulumi:"publicKey" provider:"replaceOnChanges"`
}

type SSHKeyState struct {
	SSHKeyArgs
	SSHKeyID       string `pulumi:"sshKeyId"`
	OrganizationID string `pulumi:"organizationId"`
}

func (s *SSHKeyState) Annotate(a infer.Annotator) {
	a.Describe(&s.SSHKeyID, "The stable Dokploy SSH key ID.")
	a.Describe(&s.OrganizationID, "The Dokploy organization ID owning this SSH key.")
}

type SSHKey struct{ client clientFactory }

func activeOrganizationID(org generated.Organization) string {
	if org.Id != nil && *org.Id != "" {
		return *org.Id
	}
	if org.OrganizationId != nil && *org.OrganizationId != "" {
		return *org.OrganizationId
	}
	return ""
}

func (r *SSHKey) Annotate(a infer.Annotator) {
	a.SetToken("index", "SSHKey")
	a.Describe(&r, "A Dokploy SSH key for Git and registry access.")
}

func (a *SSHKeyArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The SSH key name.")
	annotator.Describe(&a.Description, "The optional SSH key description.")
	annotator.Describe(&a.PrivateKey, "The private SSH key material.")
	annotator.Describe(&a.PublicKey, "The public SSH key material.")
}

func (r SSHKey) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[SSHKeyArgs], error) {
	in, failures, err := infer.DefaultCheck[SSHKeyArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[SSHKeyArgs]{Inputs: in, Failures: failures}, err
	}
	for _, field := range []struct{ name, value string }{
		{"name", in.Name}, {"privateKey", in.PrivateKey}, {"publicKey", in.PublicKey},
	} {
		if field.value == "" && !req.NewInputs.Get(field.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: fmt.Sprintf("%s must not be empty", field.name)})
		}
	}
	return infer.CheckResponse[SSHKeyArgs]{Inputs: in, Failures: failures}, nil
}

func (r SSHKey) Diff(_ context.Context, req infer.DiffRequest[SSHKeyArgs, SSHKeyState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.SSHKeyArgs
	d := map[string]p.PropertyDiff{}
	if in.Name != old.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(in.Description, old.Description) {
		d["description"] = p.PropertyDiff{Kind: p.Update}
	}
	if in.PrivateKey != old.PrivateKey {
		d["privateKey"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.PublicKey != old.PublicKey {
		d["publicKey"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r SSHKey) Create(ctx context.Context, req infer.CreateRequest[SSHKeyArgs]) (infer.CreateResponse[SSHKeyState], error) {
	state := SSHKeyState{SSHKeyArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[SSHKeyState]{Output: state}, nil
	}
	api := r.client(ctx)
	before, err := api.SshKeyAllWithResponse(ctx)
	if err != nil {
		return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(err, req.Inputs)
	}
	if before.JSON200 == nil {
		return infer.CreateResponse[SSHKeyState]{}, fmt.Errorf("sshKey.all returned incomplete SSH key list")
	}
	existingIDs := make(map[string]struct{}, len(*before.JSON200))
	for _, key := range *before.JSON200 {
		if key.SshKeyId != "" {
			existingIDs[key.SshKeyId] = struct{}{}
		}
	}
	org, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(err, req.Inputs)
	}
	if org.JSON200 == nil {
		return infer.CreateResponse[SSHKeyState]{}, fmt.Errorf("organization.active returned incomplete organization")
	}
	state.OrganizationID = activeOrganizationID(*org.JSON200)
	if state.OrganizationID == "" {
		return infer.CreateResponse[SSHKeyState]{}, fmt.Errorf("organization.active returned incomplete organization")
	}
	description := nullable.NewNullNullable[string]()
	if req.Inputs.Description != nil {
		description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	_, err = api.SshKeyCreateWithResponse(ctx, generated.SshKeyCreateJSONRequestBody{
		Name: req.Inputs.Name, Description: description, OrganizationId: state.OrganizationID,
		PrivateKey: req.Inputs.PrivateKey, PublicKey: req.Inputs.PublicKey,
	})
	createErr := err
	after, err := api.SshKeyAllWithResponse(ctx)
	if err != nil {
		if createErr != nil {
			return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(createErr, req.Inputs)
		}
		return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(err, req.Inputs)
	}
	if after.JSON200 == nil {
		if createErr != nil {
			return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(createErr, req.Inputs)
		}
		return infer.CreateResponse[SSHKeyState]{}, fmt.Errorf("sshKey.all returned incomplete SSH key list")
	}
	var candidates []string
	for _, key := range *after.JSON200 {
		if key.Name != nil && *key.Name == req.Inputs.Name && key.SshKeyId != "" {
			if _, exists := existingIDs[key.SshKeyId]; !exists {
				candidates = append(candidates, key.SshKeyId)
			}
		}
	}
	if len(candidates) == 0 {
		if createErr != nil {
			return infer.CreateResponse[SSHKeyState]{}, sanitizeSSHKeyError(createErr, req.Inputs)
		}
		return infer.CreateResponse[SSHKeyState]{}, initFailed(fmt.Errorf("sshKey.create returned no discoverable SSH key"))
	}
	if len(candidates) != 1 {
		return infer.CreateResponse[SSHKeyState]{}, initFailed(fmt.Errorf("sshKey.create returned ambiguous SSH key discovery"))
	}
	state.SSHKeyID = candidates[0]
	if createErr != nil {
		read, readErr := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: state.SSHKeyID, State: state})
		if readErr == nil && read.ID != "" {
			state = read.State
		}
		return infer.CreateResponse[SSHKeyState]{ID: state.SSHKeyID, Output: state}, initFailed(sanitizeSSHKeyError(createErr, req.Inputs))
	}
	read, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: state.SSHKeyID, State: state})
	if err != nil {
		return infer.CreateResponse[SSHKeyState]{ID: state.SSHKeyID, Output: state}, initFailed(sanitizeSSHKeyError(err, req.Inputs))
	}
	if read.ID == "" {
		return infer.CreateResponse[SSHKeyState]{ID: state.SSHKeyID, Output: state}, initFailed(fmt.Errorf("sshKey.one returned not found after create"))
	}
	return infer.CreateResponse[SSHKeyState]{ID: state.SSHKeyID, Output: read.State}, nil
}

func sanitizeSSHKeyError(err error, args SSHKeyArgs, prior ...SSHKeyArgs) error {
	secrets := []string{args.PrivateKey, args.PublicKey}
	for _, old := range prior {
		secrets = append(secrets, old.PrivateKey, old.PublicKey)
	}
	return sanitizeError(err, secrets...)
}

func (r SSHKey) Read(ctx context.Context, req infer.ReadRequest[SSHKeyArgs, SSHKeyState]) (infer.ReadResponse[SSHKeyArgs, SSHKeyState], error) {
	resp, err := r.client(ctx).SshKeyOneWithResponse(ctx, &generated.SshKeyOneParams{SshKeyId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[SSHKeyArgs, SSHKeyState]{ID: ""}, nil
		}
		return infer.ReadResponse[SSHKeyArgs, SSHKeyState]{}, sanitizeSSHKeyError(err, req.State.SSHKeyArgs)
	}
	if resp.JSON200 == nil || resp.JSON200.SshKeyId == "" {
		return infer.ReadResponse[SSHKeyArgs, SSHKeyState]{}, fmt.Errorf("sshKey.one returned incomplete SSH key")
	}
	v := resp.JSON200
	a := req.State.SSHKeyArgs
	a.Name = value(v.Name)
	a.Description = nullableValue(v.Description)
	if v.PrivateKey != nil && *v.PrivateKey != "" {
		a.PrivateKey = *v.PrivateKey
	}
	if v.PublicKey != nil && *v.PublicKey != "" {
		a.PublicKey = *v.PublicKey
	}
	state := SSHKeyState{SSHKeyArgs: a, SSHKeyID: v.SshKeyId, OrganizationID: value(v.OrganizationId)}
	return infer.ReadResponse[SSHKeyArgs, SSHKeyState]{ID: v.SshKeyId, Inputs: a, State: state}, nil
}

func nullableValue(v nullable.Nullable[string]) *string {
	if v.IsNull() {
		return nil
	}
	value, err := v.Get()
	if err != nil {
		return nil
	}
	return &value
}

func (r SSHKey) Update(ctx context.Context, req infer.UpdateRequest[SSHKeyArgs, SSHKeyState]) (infer.UpdateResponse[SSHKeyState], error) {
	state := SSHKeyState{SSHKeyArgs: req.Inputs, SSHKeyID: req.ID, OrganizationID: req.State.OrganizationID}
	if req.DryRun {
		return infer.UpdateResponse[SSHKeyState]{Output: state}, nil
	}
	description := nullable.NewNullNullable[string]()
	if req.Inputs.Description != nil {
		description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	_, err := r.client(ctx).SshKeyUpdateWithResponse(ctx, generated.SshKeyUpdateJSONRequestBody{SshKeyId: req.ID, Name: &req.Inputs.Name, Description: description})
	if err != nil {
		return infer.UpdateResponse[SSHKeyState]{Output: state}, sanitizeSSHKeyError(err, req.Inputs, req.State.SSHKeyArgs)
	}
	return infer.UpdateResponse[SSHKeyState]{Output: state}, nil
}

func (r SSHKey) Delete(ctx context.Context, req infer.DeleteRequest[SSHKeyState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).SshKeyRemoveWithResponse(ctx, generated.SshKeyRemoveJSONRequestBody{SshKeyId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, sanitizeSSHKeyError(err, req.State.SSHKeyArgs)
	}
	return infer.DeleteResponse{}, nil
}

func (r SSHKey) WireDependencies(f infer.FieldSelector, args *SSHKeyArgs, state *SSHKeyState) {
	f.OutputField(&state.SSHKeyID).DependsOn(f.InputField(&args.Name), f.InputField(&args.Description), f.InputField(&args.PrivateKey).Secret(), f.InputField(&args.PublicKey))
	f.OutputField(&state.OrganizationID).DependsOn(f.InputField(&args.Name))
}
