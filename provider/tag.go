package dokploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type TagArgs struct {
	Name  string  `pulumi:"name"`
	Color *string `pulumi:"color,optional"`
}

type TagState struct {
	TagArgs
	TagID string `pulumi:"tagId"`
}

func (s *TagState) Annotate(a infer.Annotator) { a.Describe(&s.TagID, "The stable Dokploy tag ID.") }

type Tag struct{ client clientFactory }

func (r *Tag) Annotate(a infer.Annotator) {
	a.SetToken("index", "Tag")
	a.Describe(&r, "A reusable Dokploy project tag.")
}
func (a *TagArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "The tag name.")
	n.Describe(&a.Color, "The optional opaque tag color.")
}

func (r Tag) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[TagArgs], error) {
	in, failures, err := infer.DefaultCheck[TagArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[TagArgs]{Inputs: in, Failures: failures}, err
	}
	if in.Name == "" && !req.NewInputs.Get("name").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	return infer.CheckResponse[TagArgs]{Inputs: in, Failures: failures}, nil
}

func (r Tag) Diff(_ context.Context, req infer.DiffRequest[TagArgs, TagState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.Name != req.State.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Color, req.State.Color) {
		d["color"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func tagColor(v *string) nullable.Nullable[string] {
	if v == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
}

func (r Tag) Create(ctx context.Context, req infer.CreateRequest[TagArgs]) (infer.CreateResponse[TagState], error) {
	state := TagState{TagArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[TagState]{Output: state}, nil
	}
	resp, err := r.client(ctx).TagCreateWithResponse(ctx, generated.TagCreateJSONRequestBody{Name: req.Inputs.Name, Color: tagColor(req.Inputs.Color)})
	if err != nil {
		return infer.CreateResponse[TagState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.TagId == "" {
		return infer.CreateResponse[TagState]{}, errors.New("tag.create returned incomplete tag")
	}
	state.TagID = resp.JSON200.TagId
	read, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: state.TagID, State: state})
	if err != nil {
		return infer.CreateResponse[TagState]{ID: state.TagID, Output: state}, initFailed(err)
	}
	if read.ID == "" {
		return infer.CreateResponse[TagState]{ID: state.TagID, Output: state}, initFailed(errors.New("tag.one returned not found after create"))
	}
	return infer.CreateResponse[TagState]{ID: state.TagID, Output: read.State}, nil
}

func (r Tag) Read(ctx context.Context, req infer.ReadRequest[TagArgs, TagState]) (infer.ReadResponse[TagArgs, TagState], error) {
	resp, err := r.client(ctx).TagOneWithResponse(ctx, &generated.TagOneParams{TagId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[TagArgs, TagState]{ID: ""}, nil
		}
		return infer.ReadResponse[TagArgs, TagState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.TagId == "" {
		return infer.ReadResponse[TagArgs, TagState]{}, fmt.Errorf("tag.one returned incomplete tag")
	}
	v := resp.JSON200
	a := req.State.TagArgs
	if v.Name != nil {
		a.Name = *v.Name
	}
	a.Color = nullableValue(v.Color)
	return infer.ReadResponse[TagArgs, TagState]{ID: v.TagId, Inputs: a, State: TagState{TagArgs: a, TagID: v.TagId}}, nil
}

func (r Tag) Update(ctx context.Context, req infer.UpdateRequest[TagArgs, TagState]) (infer.UpdateResponse[TagState], error) {
	state := TagState{TagArgs: req.Inputs, TagID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[TagState]{Output: state}, nil
	}
	name := req.Inputs.Name
	resp, err := r.client(ctx).TagUpdateWithResponse(ctx, generated.TagUpdateJSONRequestBody{TagId: req.ID, Name: &name, Color: tagColor(req.Inputs.Color)})
	if err != nil {
		return infer.UpdateResponse[TagState]{Output: state}, err
	}
	if resp.JSON200 == nil || resp.JSON200.TagId == "" {
		return infer.UpdateResponse[TagState]{Output: state}, errors.New("tag.update returned incomplete tag")
	}
	return infer.UpdateResponse[TagState]{Output: state}, nil
}

func (r Tag) Delete(ctx context.Context, req infer.DeleteRequest[TagState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).TagRemoveWithResponse(ctx, generated.TagRemoveJSONRequestBody{TagId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Tag) WireDependencies(f infer.FieldSelector, args *TagArgs, state *TagState) {
	f.OutputField(&state.TagID).DependsOn(f.InputField(&args.Name), f.InputField(&args.Color))
}
