package main

import (
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		_ = cfg.Require("dokploy:endpoint")
		_ = cfg.RequireSecret("dokploy:apiKey")
		appHost := "app.example.invalid"
		if param := cfg.Get("appHost"); param != "" {
			appHost = param
		}
		composeHost := "compose.example.invalid"
		if param := cfg.Get("composeHost"); param != "" {
			composeHost = param
		}
		gitlabIntegration := "gitlab-integration-id"
		if param := cfg.Get("gitlabIntegration"); param != "" {
			gitlabIntegration = param
		}
		gitlabProject := 42
		if param := cfg.GetInt("gitlabProject"); param != 0 {
			gitlabProject = param
		}
		gitlabOwner := "example"
		if param := cfg.Get("gitlabOwner"); param != "" {
			gitlabOwner = param
		}
		gitlabNamespace := "platform"
		if param := cfg.Get("gitlabNamespace"); param != "" {
			gitlabNamespace = param
		}
		gitlabRepository := "application"
		if param := cfg.Get("gitlabRepository"); param != "" {
			gitlabRepository = param
		}
		gitBranch := "main"
		if param := cfg.Get("gitBranch"); param != "" {
			gitBranch = param
		}
		registryPassword := "replace-with-a-registry-password"
		if param := cfg.Get("registryPassword"); param != "" {
			registryPassword = param
		}
		databasePassword := "replace-with-a-database-password"
		if param := cfg.Get("databasePassword"); param != "" {
			databasePassword = param
		}
		redisPassword := "replace-with-a-redis-password"
		if param := cfg.Get("redisPassword"); param != "" {
			redisPassword = param
		}
		projectResource, err := dokploy.NewProject(ctx, "project", &dokploy.ProjectArgs{
			Name:        pulumi.String("dokploy-mvp"),
			Description: pulumi.String("Canonical Dokploy provider example"),
		})
		if err != nil {
			return err
		}
		environment, err := dokploy.NewEnvironment(ctx, "environment", &dokploy.EnvironmentArgs{
			ProjectId:   projectResource.ProjectId,
			Name:        pulumi.String("staging"),
			Description: pulumi.String("Additional non-production environment"),
		})
		if err != nil {
			return err
		}
		application, err := dokploy.NewApplication(ctx, "application", &dokploy.ApplicationArgs{
			Name:          pulumi.String("mvp-application"),
			EnvironmentId: projectResource.DefaultEnvironmentId,
			Source: &dokploy.ApplicationSourceArgs{
				Type: pulumi.String("docker"),
				Docker: &dokploy.DockerSourceArgs{
					Image:    pulumi.String("nginx:1.27"),
					Username: pulumi.String("example"),
					Password: pulumi.ToSecret(registryPassword).(pulumi.StringOutput),
				},
			},
			Environment:   pulumi.ToSecret(fmt.Sprintf("APP_HOST=%v", appHost)).(pulumi.StringOutput),
			CreateEnvFile: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		compose, err := dokploy.NewCompose(ctx, "compose", &dokploy.ComposeArgs{
			Name:          pulumi.String("mvp-compose"),
			EnvironmentId: projectResource.DefaultEnvironmentId,
			Source: &dokploy.ComposeSourceArgs{
				Type: pulumi.String("raw"),
				Raw: &dokploy.RawComposeSourceArgs{
					ComposeFile: pulumi.String(`services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
`),
				},
			},
			Environment:   pulumi.ToSecret(fmt.Sprintf("COMPOSE_HOST=%v", composeHost)).(pulumi.StringOutput),
			CreateEnvFile: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewPostgres(ctx, "postgres", &dokploy.PostgresArgs{
			Name:             pulumi.String("mvp-postgres"),
			EnvironmentId:    environment.EnvironmentId,
			DatabaseName:     pulumi.String("app"),
			DatabaseUser:     pulumi.String("app"),
			DatabasePassword: pulumi.ToSecret(databasePassword).(pulumi.StringOutput),
			Environment:      pulumi.ToSecret("POSTGRES_HOST=postgres").(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewRedis(ctx, "redis", &dokploy.RedisArgs{
			Name:             pulumi.String("mvp-redis"),
			EnvironmentId:    environment.EnvironmentId,
			DatabasePassword: pulumi.ToSecret(redisPassword).(pulumi.StringOutput),
			Environment:      pulumi.ToSecret("REDIS_HOST=redis").(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewDomain(ctx, "applicationDomain", &dokploy.DomainArgs{
			ApplicationId:   application.ApplicationId,
			Host:            pulumi.String(appHost),
			Port:            pulumi.Int(80),
			Https:           pulumi.Bool(true),
			CertificateType: pulumi.String("none"),
			Enabled:         pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewDomain(ctx, "composeDomain", &dokploy.DomainArgs{
			ComposeId:       compose.ComposeId,
			ServiceName:     pulumi.String("web"),
			Host:            pulumi.String(composeHost),
			Port:            pulumi.Int(80),
			Https:           pulumi.Bool(true),
			CertificateType: pulumi.String("none"),
			Enabled:         pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		ctx.Export("gitlabIntegration", pulumi.String(gitlabIntegration))
		ctx.Export("gitlabProject", pulumi.Int(gitlabProject))
		ctx.Export("gitlabOwner", pulumi.String(gitlabOwner))
		ctx.Export("gitlabNamespace", pulumi.String(gitlabNamespace))
		ctx.Export("gitlabRepository", pulumi.String(gitlabRepository))
		ctx.Export("gitBranch", pulumi.String(gitBranch))
		return nil
	})
}
