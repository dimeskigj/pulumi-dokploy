package tests

import (
	"context"
	"path/filepath"
	"testing"

	dokploy "github.com/gjorgjidimeski/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type liveConfig struct {
	Endpoint   string
	APIKey     string
	NameSuffix string
}

func runMVP(t *testing.T, ctx context.Context, cfg liveConfig) {
	t.Helper()
	backend := t.TempDir()
	program := mvpProgram(cfg, "nginx:1.27")
	stack, err := auto.NewStackInlineSource(ctx, "mvp-"+cfg.NameSuffix, "dokploy-mvp", program,
		auto.PulumiHome(filepath.Join(backend, "pulumi")),
		auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": "file://" + backend}),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Register both cleanup actions immediately after stack creation.
	t.Cleanup(func() {
		if _, err := stack.Destroy(context.Background()); err != nil {
			t.Errorf("destroy live MVP stack: %v", err)
		}
		if err := stack.Workspace().RemoveStack(context.Background(), "mvp-"+cfg.NameSuffix); err != nil {
			t.Errorf("remove live MVP workspace: %v", err)
		}
	})
	if err := stack.SetConfig(ctx, "dokploy:endpoint", auto.ConfigValue{Value: cfg.Endpoint}); err != nil {
		t.Fatal(err)
	}
	if err := stack.SetConfig(ctx, "dokploy:apiKey", auto.ConfigValue{Value: cfg.APIKey, Secret: true}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"databasePassword", "redisPassword", "applicationSecret", "composeSecret"} {
		if err := stack.SetConfig(ctx, key, auto.ConfigValue{Value: "task12-" + key, Secret: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := stack.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	stack.Workspace().SetProgram(mvpProgram(cfg, "nginx:1.28"))
	if _, err := stack.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

func mvpProgram(cfg liveConfig, image string) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		project, err := dokploy.NewProject(ctx, "project-"+cfg.NameSuffix, &dokploy.ProjectArgs{Name: pulumi.String("task12-" + cfg.NameSuffix)})
		if err != nil {
			return err
		}
		environment, err := dokploy.NewEnvironment(ctx, "environment-"+cfg.NameSuffix, &dokploy.EnvironmentArgs{ProjectId: project.ProjectId, Name: pulumi.String("staging")})
		if err != nil {
			return err
		}
		application, err := dokploy.NewApplication(ctx, "application-"+cfg.NameSuffix, &dokploy.ApplicationArgs{Name: pulumi.String("application-" + cfg.NameSuffix), EnvironmentId: project.DefaultEnvironmentId, Source: &dokploy.ApplicationSourceArgs{Type: pulumi.String("docker"), Docker: &dokploy.DockerSourceArgs{Image: pulumi.String(image)}}, Environment: pulumi.ToSecret(pulumi.StringPtr("APP_SECRET=task12")).(pulumi.StringPtrOutput)})
		if err != nil {
			return err
		}
		compose, err := dokploy.NewCompose(ctx, "compose-"+cfg.NameSuffix, &dokploy.ComposeArgs{Name: pulumi.String("compose-" + cfg.NameSuffix), EnvironmentId: project.DefaultEnvironmentId, Source: &dokploy.ComposeSourceArgs{Type: pulumi.String("raw"), Raw: &dokploy.RawComposeSourceArgs{ComposeFile: pulumi.String("services:\n  web:\n    image: nginx:1.27\n")}}})
		if err != nil {
			return err
		}
		_, err = dokploy.NewPostgres(ctx, "postgres-"+cfg.NameSuffix, &dokploy.PostgresArgs{Name: pulumi.String("postgres-" + cfg.NameSuffix), EnvironmentId: environment.EnvironmentId, DatabaseName: pulumi.String("app"), DatabaseUser: pulumi.String("app"), DatabasePassword: pulumi.ToSecret(pulumi.String("task12-db")).(pulumi.StringOutput)})
		if err != nil {
			return err
		}
		_, err = dokploy.NewRedis(ctx, "redis-"+cfg.NameSuffix, &dokploy.RedisArgs{Name: pulumi.String("redis-" + cfg.NameSuffix), EnvironmentId: environment.EnvironmentId, DatabasePassword: pulumi.ToSecret(pulumi.String("task12-redis")).(pulumi.StringOutput)})
		if err != nil {
			return err
		}
		_, err = dokploy.NewDomain(ctx, "application-domain-"+cfg.NameSuffix, &dokploy.DomainArgs{ApplicationId: application.ApplicationId, Host: pulumi.String("app-" + cfg.NameSuffix + ".example.invalid"), Port: pulumi.Int(80), CertificateType: pulumi.String("none")})
		if err != nil {
			return err
		}
		_, err = dokploy.NewDomain(ctx, "compose-domain-"+cfg.NameSuffix, &dokploy.DomainArgs{ComposeId: compose.ComposeId, ServiceName: pulumi.String("web"), Host: pulumi.String("compose-" + cfg.NameSuffix + ".example.invalid"), Port: pulumi.Int(80), CertificateType: pulumi.String("none")})
		return err
	}
}
