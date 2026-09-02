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
		sshPrivateKey := "replace-with-an-ssh-private-key"
		if param := cfg.Get("sshPrivateKey"); param != "" {
			sshPrivateKey = param
		}
		registryPassword := "replace-with-a-registry-password"
		if param := cfg.Get("registryPassword"); param != "" {
			registryPassword = param
		}
		databasePassword := "replace-with-a-database-password"
		if param := cfg.Get("databasePassword"); param != "" {
			databasePassword = param
		}
		mysqlPassword := "replace-with-a-mysql-password"
		if param := cfg.Get("mysqlPassword"); param != "" {
			mysqlPassword = param
		}
		mariadbPassword := "replace-with-a-mariadb-password"
		if param := cfg.Get("mariadbPassword"); param != "" {
			mariadbPassword = param
		}
		mongodbPassword := "replace-with-a-mongodb-password"
		if param := cfg.Get("mongodbPassword"); param != "" {
			mongodbPassword = param
		}
		redisPassword := "replace-with-a-redis-password"
		if param := cfg.Get("redisPassword"); param != "" {
			redisPassword = param
		}
		destinationAccessKey := "replace-with-a-destination-access-key"
		if param := cfg.Get("destinationAccessKey"); param != "" {
			destinationAccessKey = param
		}
		destinationSecretAccessKey := "replace-with-a-destination-secret-access-key"
		if param := cfg.Get("destinationSecretAccessKey"); param != "" {
			destinationSecretAccessKey = param
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
		registry, err := dokploy.NewRegistry(ctx, "registry", &dokploy.RegistryArgs{
			Name:        pulumi.String("mvp-registry"),
			Username:    pulumi.String("example"),
			Password:    pulumi.ToSecret(registryPassword).(pulumi.StringOutput),
			Url:         pulumi.String("registry.example.invalid"),
			ImagePrefix: pulumi.String("dokploy/"),
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
			Environment:     pulumi.ToSecret(fmt.Sprintf("APP_HOST=%v", appHost)).(pulumi.StringOutput),
			CreateEnvFile:   pulumi.Bool(true),
			RegistryId:      registry.RegistryId,
			BuildRegistryId: registry.RegistryId,
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
		_, err = dokploy.NewSSHKey(ctx, "sshKey", &dokploy.SSHKeyArgs{
			Name:       pulumi.String("mvp-git-ssh"),
			PrivateKey: pulumi.ToSecret(sshPrivateKey).(pulumi.StringOutput),
			PublicKey:  pulumi.String("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample dokploy-mvp"),
		})
		if err != nil {
			return err
		}
		tag, err := dokploy.NewTag(ctx, "tag", &dokploy.TagArgs{
			Name:  pulumi.String("mvp"),
			Color: pulumi.String("#2dd4bf"),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewProjectTag(ctx, "projectTag", &dokploy.ProjectTagArgs{
			ProjectId: projectResource.ProjectId,
			TagId:     tag.TagId,
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewMount(ctx, "applicationBindMount", &dokploy.MountArgs{
			Type:          pulumi.String("bind"),
			MountPath:     pulumi.String("/var/lib/dokploy"),
			HostPath:      pulumi.String("/srv/dokploy"),
			ApplicationId: application.ApplicationId,
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewMount(ctx, "composeVolumeMount", &dokploy.MountArgs{
			Type:       pulumi.String("volume"),
			MountPath:  pulumi.String("/var/lib/postgresql/data"),
			VolumeName: pulumi.String("mvp-postgres-data"),
			ComposeId:  compose.ComposeId,
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewMount(ctx, "postgresFileMount", &dokploy.MountArgs{
			Type:      pulumi.String("file"),
			MountPath: pulumi.String("/etc/app/config.toml"),
			FilePath:  pulumi.String("/tmp/dokploy-config.toml"),
			Content:   pulumi.ToSecret("APP_ENV=staging").(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		postgres, err := dokploy.NewPostgres(ctx, "postgres", &dokploy.PostgresArgs{
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
		_, err = dokploy.NewMySQL(ctx, "mysql", &dokploy.MySQLArgs{
			Name:             pulumi.String("mvp-mysql"),
			EnvironmentId:    environment.EnvironmentId,
			DatabaseName:     pulumi.String("app"),
			DatabaseUser:     pulumi.String("app"),
			DatabasePassword: pulumi.ToSecret(mysqlPassword).(pulumi.StringOutput),
			Environment:      pulumi.ToSecret("MYSQL_HOST=mysql").(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewMariaDB(ctx, "mariadb", &dokploy.MariaDBArgs{
			Name:             pulumi.String("mvp-mariadb"),
			EnvironmentId:    environment.EnvironmentId,
			DatabaseName:     pulumi.String("app"),
			DatabaseUser:     pulumi.String("app"),
			DatabasePassword: pulumi.ToSecret(mariadbPassword).(pulumi.StringOutput),
			Environment:      pulumi.ToSecret("MARIADB_HOST=mariadb").(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewMongoDB(ctx, "mongodb", &dokploy.MongoDBArgs{
			Name:             pulumi.String("mvp-mongodb"),
			EnvironmentId:    environment.EnvironmentId,
			DatabaseUser:     pulumi.String("app"),
			DatabasePassword: pulumi.ToSecret(mongodbPassword).(pulumi.StringOutput),
			Environment:      pulumi.ToSecret("MONGODB_HOST=mongodb").(pulumi.StringOutput),
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
		destination, err := dokploy.NewDestination(ctx, "destination", &dokploy.DestinationArgs{
			Name:            pulumi.String("mvp-destination"),
			Provider:        pulumi.String("s3"),
			AccessKey:       pulumi.String(destinationAccessKey),
			SecretAccessKey: pulumi.ToSecret(destinationSecretAccessKey).(pulumi.StringOutput),
			Bucket:          pulumi.String("dokploy-mvp-backups"),
			Region:          pulumi.String("us-east-1"),
			Endpoint:        pulumi.String("https://s3.us-east-1.amazonaws.com"),
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewBackup(ctx, "postgresBackup", &dokploy.BackupArgs{
			Schedule:      pulumi.String("0 0 * * *"),
			Prefix:        pulumi.String("postgres-"),
			DestinationId: destination.DestinationId,
			Database:      pulumi.String("app"),
			PostgresId:    postgres.PostgresId,
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewVolumeBackup(ctx, "applicationVolumeBackup", &dokploy.VolumeBackupArgs{
			Name:           pulumi.String("mvp-application-volume-backup"),
			VolumeName:     pulumi.String("mvp-application-data"),
			Prefix:         pulumi.String("application-"),
			DestinationId:  destination.DestinationId,
			CronExpression: pulumi.String("0 0 * * *"),
			ApplicationId:  application.ApplicationId,
		})
		if err != nil {
			return err
		}
		_, err = dokploy.NewVolumeBackup(ctx, "composeVolumeBackup", &dokploy.VolumeBackupArgs{
			Name:           pulumi.String("mvp-compose-volume-backup"),
			VolumeName:     pulumi.String("mvp-compose-data"),
			Prefix:         pulumi.String("compose-"),
			DestinationId:  destination.DestinationId,
			CronExpression: pulumi.String("0 0 * * *"),
			ComposeId:      compose.ComposeId,
			ServiceName:    pulumi.String("web"),
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
