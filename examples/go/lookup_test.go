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
	case "dokploy:index:getServer":
		if args.Args["serverId"].StringValue() != "server-example" {
			return nil, fmt.Errorf("unexpected server invoke input")
		}
		return resource.NewPropertyMapFromMap(map[string]any{
			"serverId": "server-example", "name": "Existing server",
		}), nil
	default:
		return nil, fmt.Errorf("unexpected invoke token: %s", args.Token)
	}
}

type serverLookupMocks struct {
	lookupMocks
	reads int
}

func (m *serverLookupMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken != "dokploy:index:Server" {
		return m.lookupMocks.NewResource(args)
	}
	if args.ReadRPC == nil || args.Name != "managedServer" || args.ID != "server-example" {
		return "", nil, fmt.Errorf("unexpected managed Server read")
	}
	m.reads++
	return args.ID, resource.NewPropertyMapFromMap(map[string]any{
		"serverId": args.ID, "name": "Managed server",
	}), nil
}

func TestGoServerInvokeAndManagedGetterCoexist(t *testing.T) {
	// Compile-time checks keep both the invoke and managed-resource getter usable.
	var _ func(*pulumi.Context, *dokploy.LookupServerArgs, ...pulumi.InvokeOption) (*dokploy.LookupServerResult, error) = dokploy.LookupServer
	var _ func(*pulumi.Context, dokploy.LookupServerOutputArgs, ...pulumi.InvokeOption) dokploy.LookupServerResultOutput = dokploy.LookupServerOutput
	var _ func(*pulumi.Context, string, pulumi.IDInput, *dokploy.ServerState, ...pulumi.ResourceOption) (*dokploy.Server, error) = dokploy.GetServer

	mocks := &serverLookupMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		plain, err := dokploy.LookupServer(ctx, &dokploy.LookupServerArgs{ServerId: "server-example"})
		if err != nil {
			return err
		}
		if plain.ServerId != "server-example" || plain.Name != "Existing server" {
			return fmt.Errorf("plain Server invoke metadata mismatch")
		}
		output := dokploy.LookupServerOutput(ctx, dokploy.LookupServerOutputArgs{ServerId: pulumi.String("server-example")})
		ctx.Export("lookupName", output.Name().ApplyT(func(name string) (string, error) {
			if name != "Existing server" {
				return "", fmt.Errorf("output Server invoke metadata mismatch")
			}
			return name, nil
		}))
		managed, err := dokploy.GetServer(ctx, "managedServer", pulumi.ID("server-example"), nil)
		if err != nil {
			return err
		}
		ctx.Export("managedName", managed.Name.ApplyT(func(name string) (string, error) {
			if name != "Managed server" {
				return "", fmt.Errorf("managed Server read metadata mismatch")
			}
			return name, nil
		}))
		return nil
	}, pulumi.WithMocks("server-lookup-example", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if len(mocks.calls) != 2 || mocks.calls[0] != "dokploy:index:getServer" || mocks.calls[1] != "dokploy:index:getServer" {
		t.Fatalf("unexpected invoke tokens: %v", mocks.calls)
	}
	if mocks.reads != 1 || mocks.resources != 0 {
		t.Fatalf("unexpected resource reads=%d registrations=%d", mocks.reads, mocks.resources)
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
