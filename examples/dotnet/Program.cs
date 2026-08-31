using System.Collections.Generic;
using System.Linq;
using Pulumi;
using Dokploy = Pulumi.Dokploy;

return await Deployment.RunAsync(() =>
{
    var config = new Config();
    var dokployEndpoint = config.Require("dokploy:endpoint");
    var dokployApiKey = config.RequireSecret("dokploy:apiKey");
    var appHost = config.Get("appHost") ?? "app.example.invalid";
    var composeHost = config.Get("composeHost") ?? "compose.example.invalid";
    var gitlabIntegration = config.Get("gitlabIntegration") ?? "gitlab-integration-id";
    var gitlabProject = config.GetInt32("gitlabProject") ?? 42;
    var gitlabOwner = config.Get("gitlabOwner") ?? "example";
    var gitlabNamespace = config.Get("gitlabNamespace") ?? "platform";
    var gitlabRepository = config.Get("gitlabRepository") ?? "application";
    var gitBranch = config.Get("gitBranch") ?? "main";
    var registryPassword = config.GetSecret("registryPassword") ?? Output.CreateSecret("replace-with-a-registry-password");
    var databasePassword = config.GetSecret("databasePassword") ?? Output.CreateSecret("replace-with-a-database-password");
    var mysqlPassword = config.GetSecret("mysqlPassword") ?? Output.CreateSecret("replace-with-a-mysql-password");
    var mariadbPassword = config.GetSecret("mariadbPassword") ?? Output.CreateSecret("replace-with-a-mariadb-password");
    var mongodbPassword = config.GetSecret("mongodbPassword") ?? Output.CreateSecret("replace-with-a-mongodb-password");
    var redisPassword = config.GetSecret("redisPassword") ?? Output.CreateSecret("replace-with-a-redis-password");
    var destinationAccessKey = config.Get("destinationAccessKey") ?? "replace-with-a-destination-access-key";
    var destinationSecretAccessKey = config.GetSecret("destinationSecretAccessKey") ?? Output.CreateSecret("replace-with-a-destination-secret-access-key");
    var projectResource = new Dokploy.Project("project", new()
    {
        Name = "dokploy-mvp",
        Description = "Canonical Dokploy provider example",
    });

    var environment = new Dokploy.Environment("environment", new()
    {
        ProjectId = projectResource.ProjectId,
        Name = "staging",
        Description = "Additional non-production environment",
    });

    var application = new Dokploy.Application("application", new()
    {
        Name = "mvp-application",
        EnvironmentId = projectResource.DefaultEnvironmentId,
        Source = new Dokploy.Inputs.ApplicationSourceArgs
        {
            Type = "docker",
            Docker = new Dokploy.Inputs.DockerSourceArgs
            {
                Image = "nginx:1.27",
                Username = "example",
                Password = Output.CreateSecret(registryPassword),
            },
        },
        Environment = Output.CreateSecret($"APP_HOST={appHost}"),
        CreateEnvFile = true,
    });

    var compose = new Dokploy.Compose("compose", new()
    {
        Name = "mvp-compose",
        EnvironmentId = projectResource.DefaultEnvironmentId,
        Source = new Dokploy.Inputs.ComposeSourceArgs
        {
            Type = "raw",
            Raw = new Dokploy.Inputs.RawComposeSourceArgs
            {
                ComposeFile = @"services:
  web:
    image: nginx:1.27
    ports:
      - ""8080:80""
",
            },
        },
        Environment = Output.CreateSecret($"COMPOSE_HOST={composeHost}"),
        CreateEnvFile = true,
    });

    var postgres = new Dokploy.Postgres("postgres", new()
    {
        Name = "mvp-postgres",
        EnvironmentId = environment.EnvironmentId,
        DatabaseName = "app",
        DatabaseUser = "app",
        DatabasePassword = Output.CreateSecret(databasePassword),
        Environment = Output.CreateSecret("POSTGRES_HOST=postgres"),
    });

    var mysql = new Dokploy.MySQL("mysql", new()
    {
        Name = "mvp-mysql",
        EnvironmentId = environment.EnvironmentId,
        DatabaseName = "app",
        DatabaseUser = "app",
        DatabasePassword = Output.CreateSecret(mysqlPassword),
        Environment = Output.CreateSecret("MYSQL_HOST=mysql"),
    });

    var mariadb = new Dokploy.MariaDB("mariadb", new()
    {
        Name = "mvp-mariadb",
        EnvironmentId = environment.EnvironmentId,
        DatabaseName = "app",
        DatabaseUser = "app",
        DatabasePassword = Output.CreateSecret(mariadbPassword),
        Environment = Output.CreateSecret("MARIADB_HOST=mariadb"),
    });

    var mongodb = new Dokploy.MongoDB("mongodb", new()
    {
        Name = "mvp-mongodb",
        EnvironmentId = environment.EnvironmentId,
        DatabaseUser = "app",
        DatabasePassword = Output.CreateSecret(mongodbPassword),
        Environment = Output.CreateSecret("MONGODB_HOST=mongodb"),
    });

    var redis = new Dokploy.Redis("redis", new()
    {
        Name = "mvp-redis",
        EnvironmentId = environment.EnvironmentId,
        DatabasePassword = Output.CreateSecret(redisPassword),
        Environment = Output.CreateSecret("REDIS_HOST=redis"),
    });

    var destination = new Dokploy.Destination("destination", new()
    {
        Name = "mvp-destination",
        Provider = "s3",
        AccessKey = destinationAccessKey,
        SecretAccessKey = Output.CreateSecret(destinationSecretAccessKey),
        Bucket = "dokploy-mvp-backups",
        Region = "us-east-1",
        Endpoint = "https://s3.us-east-1.amazonaws.com",
    });

    var postgresBackup = new Dokploy.Backup("postgresBackup", new()
    {
        Schedule = "0 0 * * *",
        Prefix = "postgres-",
        DestinationId = destination.DestinationId,
        Database = "app",
        PostgresId = postgres.PostgresId,
    });

    var applicationVolumeBackup = new Dokploy.VolumeBackup("applicationVolumeBackup", new()
    {
        Name = "mvp-application-volume-backup",
        VolumeName = "mvp-application-data",
        Prefix = "application-",
        DestinationId = destination.DestinationId,
        CronExpression = "0 0 * * *",
        ApplicationId = application.ApplicationId,
    });

    var composeVolumeBackup = new Dokploy.VolumeBackup("composeVolumeBackup", new()
    {
        Name = "mvp-compose-volume-backup",
        VolumeName = "mvp-compose-data",
        Prefix = "compose-",
        DestinationId = destination.DestinationId,
        CronExpression = "0 0 * * *",
        ComposeId = compose.ComposeId,
        ServiceName = "web",
    });

    var applicationDomain = new Dokploy.Domain("applicationDomain", new()
    {
        ApplicationId = application.ApplicationId,
        Host = appHost,
        Port = 80,
        Https = true,
        CertificateType = "none",
        Enabled = true,
    });

    var composeDomain = new Dokploy.Domain("composeDomain", new()
    {
        ComposeId = compose.ComposeId,
        ServiceName = "web",
        Host = composeHost,
        Port = 80,
        Https = true,
        CertificateType = "none",
        Enabled = true,
    });

    return new Dictionary<string, object?>
    {
        ["gitlabIntegration"] = gitlabIntegration,
        ["gitlabProject"] = gitlabProject,
        ["gitlabOwner"] = gitlabOwner,
        ["gitlabNamespace"] = gitlabNamespace,
        ["gitlabRepository"] = gitlabRepository,
        ["gitBranch"] = gitBranch,
    };
});
