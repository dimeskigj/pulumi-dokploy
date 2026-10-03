package main

import (
	"fmt"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type lookupMocks struct {
	calls     []string
	resources int
}

func (m *lookupMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken != "pulumi:pulumi:Stack" {
		m.resources++
		return "", nil, fmt.Errorf("unexpected resource registration: %s", args.TypeToken)
	}
	return "stack", args.Inputs, nil
}

func (m *lookupMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	m.calls = append(m.calls, args.Token)
	switch args.Token {
	case "dokploy:index:getProject":
		if args.Args["projectId"].StringValue() != "project-example" {
			return nil, fmt.Errorf("unexpected project invoke input")
		}
		return resource.NewPropertyMapFromMap(map[string]any{
			"projectId": "project-example", "name": "Existing project",
			"defaultEnvironmentId": "environment-example",
		}), nil
	case "dokploy:index:getEnvironment":
		if args.Args["environmentId"].StringValue() != "environment-example" {
			return nil, fmt.Errorf("unexpected environment invoke input")
		}
		return resource.NewPropertyMapFromMap(map[string]any{
			"environmentId": "environment-example", "name": "Existing environment",
			"projectId": "project-example", "isDefault": true,
		}), nil
	default:
		return nil, fmt.Errorf("unexpected invoke token: %s", args.Token)
	}
}

func TestLookupSDKReferenceWithoutResourceRegistration(t *testing.T) {
	// Compile-time check: the generated output-form invoke exposes optional typed metadata.
	var _ func(dokploy.LookupProjectResultOutput) pulumi.StringPtrOutput = dokploy.LookupProjectResultOutput.DefaultEnvironmentId
	var _ func(dokploy.LookupEnvironmentResultOutput) pulumi.BoolPtrOutput = dokploy.LookupEnvironmentResultOutput.IsDefault

	mocks := &lookupMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		project, err := dokploy.LookupProject(ctx, &dokploy.LookupProjectArgs{ProjectId: "project-example"})
		if err != nil {
			return err
		}
		if project.ProjectId != "project-example" || project.Name != "Existing project" || project.DefaultEnvironmentId == nil || *project.DefaultEnvironmentId != "environment-example" {
			return fmt.Errorf("project metadata mismatch")
		}
		environment, err := dokploy.LookupEnvironment(ctx, &dokploy.LookupEnvironmentArgs{EnvironmentId: "environment-example"})
		if err != nil {
			return err
		}
		if environment.EnvironmentId != "environment-example" || environment.Name != "Existing environment" || environment.ProjectId == nil || *environment.ProjectId != "project-example" || environment.IsDefault == nil || !*environment.IsDefault {
			return fmt.Errorf("environment metadata mismatch")
		}
		return nil
	}, pulumi.WithMocks("lookup-example", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if len(mocks.calls) != 2 || mocks.calls[0] != "dokploy:index:getProject" || mocks.calls[1] != "dokploy:index:getEnvironment" {
		t.Fatalf("unexpected invoke tokens: %v", mocks.calls)
	}
	if mocks.resources != 0 {
		t.Fatalf("unexpected Dokploy registrations: %d", mocks.resources)
	}
}
