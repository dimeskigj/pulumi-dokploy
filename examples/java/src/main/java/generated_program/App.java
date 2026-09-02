package generated_program;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.core.Output;
import net.dimeski.pulumi.dokploy.Project;
import net.dimeski.pulumi.dokploy.ProjectArgs;
import net.dimeski.pulumi.dokploy.Environment;
import net.dimeski.pulumi.dokploy.EnvironmentArgs;
import net.dimeski.pulumi.dokploy.Registry;
import net.dimeski.pulumi.dokploy.RegistryArgs;
import net.dimeski.pulumi.dokploy.Application;
import net.dimeski.pulumi.dokploy.ApplicationArgs;
import net.dimeski.pulumi.dokploy.inputs.ApplicationSourceArgs;
import net.dimeski.pulumi.dokploy.inputs.DockerSourceArgs;
import net.dimeski.pulumi.dokploy.Compose;
import net.dimeski.pulumi.dokploy.ComposeArgs;
import net.dimeski.pulumi.dokploy.inputs.ComposeSourceArgs;
import net.dimeski.pulumi.dokploy.inputs.RawComposeSourceArgs;
import net.dimeski.pulumi.dokploy.SSHKey;
import net.dimeski.pulumi.dokploy.SSHKeyArgs;
import net.dimeski.pulumi.dokploy.Tag;
import net.dimeski.pulumi.dokploy.TagArgs;
import net.dimeski.pulumi.dokploy.ProjectTag;
import net.dimeski.pulumi.dokploy.ProjectTagArgs;
import net.dimeski.pulumi.dokploy.Mount;
import net.dimeski.pulumi.dokploy.MountArgs;
import net.dimeski.pulumi.dokploy.Postgres;
import net.dimeski.pulumi.dokploy.PostgresArgs;
import net.dimeski.pulumi.dokploy.MySQL;
import net.dimeski.pulumi.dokploy.MySQLArgs;
import net.dimeski.pulumi.dokploy.MariaDB;
import net.dimeski.pulumi.dokploy.MariaDBArgs;
import net.dimeski.pulumi.dokploy.MongoDB;
import net.dimeski.pulumi.dokploy.MongoDBArgs;
import net.dimeski.pulumi.dokploy.Redis;
import net.dimeski.pulumi.dokploy.RedisArgs;
import net.dimeski.pulumi.dokploy.Destination;
import net.dimeski.pulumi.dokploy.DestinationArgs;
import net.dimeski.pulumi.dokploy.Backup;
import net.dimeski.pulumi.dokploy.BackupArgs;
import net.dimeski.pulumi.dokploy.VolumeBackup;
import net.dimeski.pulumi.dokploy.VolumeBackupArgs;
import net.dimeski.pulumi.dokploy.Domain;
import net.dimeski.pulumi.dokploy.DomainArgs;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Map;
import java.io.File;
import java.nio.file.Files;
import java.nio.file.Paths;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    public static void stack(Context ctx) {
        final var config = ctx.config();
        final var dokployEndpoint = config.require("dokploy:endpoint");
        final var dokployApiKey = config.requireSecret("dokploy:apiKey");
        final var appHost = config.get("appHost").orElse("app.example.invalid");
        final var composeHost = config.get("composeHost").orElse("compose.example.invalid");
        final var gitlabIntegration = config.get("gitlabIntegration").orElse("gitlab-integration-id");
        final var gitlabProject = config.getInteger("gitlabProject").orElse(42);
        final var gitlabOwner = config.get("gitlabOwner").orElse("example");
        final var gitlabNamespace = config.get("gitlabNamespace").orElse("platform");
        final var gitlabRepository = config.get("gitlabRepository").orElse("application");
        final var gitBranch = config.get("gitBranch").orElse("main");
        final var sshPrivateKey = config.getSecret("sshPrivateKey").applyValue(v -> v.orElse("replace-with-an-ssh-private-key"));
        final var registryPassword = config.getSecret("registryPassword").applyValue(v -> v.orElse("replace-with-a-registry-password"));
        final var databasePassword = config.getSecret("databasePassword").applyValue(v -> v.orElse("replace-with-a-database-password"));
        final var mysqlPassword = config.getSecret("mysqlPassword").applyValue(v -> v.orElse("replace-with-a-mysql-password"));
        final var mariadbPassword = config.getSecret("mariadbPassword").applyValue(v -> v.orElse("replace-with-a-mariadb-password"));
        final var mongodbPassword = config.getSecret("mongodbPassword").applyValue(v -> v.orElse("replace-with-a-mongodb-password"));
        final var redisPassword = config.getSecret("redisPassword").applyValue(v -> v.orElse("replace-with-a-redis-password"));
        final var destinationAccessKey = config.get("destinationAccessKey").orElse("replace-with-a-destination-access-key");
        final var destinationSecretAccessKey = config.getSecret("destinationSecretAccessKey").applyValue(v -> v.orElse("replace-with-a-destination-secret-access-key"));
        var projectResource = new Project("projectResource", ProjectArgs.builder()
            .name("dokploy-mvp")
            .description("Canonical Dokploy provider example")
            .build());

        var environment = new Environment("environment", EnvironmentArgs.builder()
            .projectId(projectResource.projectId())
            .name("staging")
            .description("Additional non-production environment")
            .build());

        var registry = new Registry("registry", RegistryArgs.builder()
            .name("mvp-registry")
            .username("example")
            .password(registryPassword.asSecret())
            .url("registry.example.invalid")
            .imagePrefix("dokploy/")
            .build());

        var application = new Application("application", ApplicationArgs.builder()
            .name("mvp-application")
            .environmentId(projectResource.defaultEnvironmentId())
            .source(ApplicationSourceArgs.builder()
                .type("docker")
                .docker(DockerSourceArgs.builder()
                    .image("nginx:1.27")
                    .username("example")
                    .password(registryPassword.asSecret())
                    .build())
                .build())
            .environment(Output.ofSecret(String.format("APP_HOST=%s", appHost)))
            .createEnvFile(true)
            .registryId(registry.registryId())
            .buildRegistryId(registry.registryId())
            .build());

        var compose = new Compose("compose", ComposeArgs.builder()
            .name("mvp-compose")
            .environmentId(projectResource.defaultEnvironmentId())
            .source(ComposeSourceArgs.builder()
                .type("raw")
                .raw(RawComposeSourceArgs.builder()
                    .composeFile("""
services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
                    """)
                    .build())
                .build())
            .environment(Output.ofSecret(String.format("COMPOSE_HOST=%s", composeHost)))
            .createEnvFile(true)
            .build());

        var sshKey = new SSHKey("sshKey", SSHKeyArgs.builder()
            .name("mvp-git-ssh")
            .privateKey(sshPrivateKey.asSecret())
            .publicKey("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample dokploy-mvp")
            .build());

        var tag = new Tag("tag", TagArgs.builder()
            .name("mvp")
            .color("#2dd4bf")
            .build());

        var projectTag = new ProjectTag("projectTag", ProjectTagArgs.builder()
            .projectId(projectResource.projectId())
            .tagId(tag.tagId())
            .build());

        var applicationBindMount = new Mount("applicationBindMount", MountArgs.builder()
            .type("bind")
            .mountPath("/var/lib/dokploy")
            .hostPath("/srv/dokploy")
            .applicationId(application.applicationId())
            .build());

        var composeVolumeMount = new Mount("composeVolumeMount", MountArgs.builder()
            .type("volume")
            .mountPath("/var/lib/postgresql/data")
            .volumeName("mvp-postgres-data")
            .composeId(compose.composeId())
            .build());

        var postgresFileMount = new Mount("postgresFileMount", MountArgs.builder()
            .type("file")
            .mountPath("/etc/app/config.toml")
            .filePath("/tmp/dokploy-config.toml")
            .content(Output.ofSecret("APP_ENV=staging"))
            .build());

        var postgres = new Postgres("postgres", PostgresArgs.builder()
            .name("mvp-postgres")
            .environmentId(environment.environmentId())
            .databaseName("app")
            .databaseUser("app")
            .databasePassword(databasePassword.asSecret())
            .environment(Output.ofSecret("POSTGRES_HOST=postgres"))
            .build());

        var mysql = new MySQL("mysql", MySQLArgs.builder()
            .name("mvp-mysql")
            .environmentId(environment.environmentId())
            .databaseName("app")
            .databaseUser("app")
            .databasePassword(mysqlPassword.asSecret())
            .environment(Output.ofSecret("MYSQL_HOST=mysql"))
            .build());

        var mariadb = new MariaDB("mariadb", MariaDBArgs.builder()
            .name("mvp-mariadb")
            .environmentId(environment.environmentId())
            .databaseName("app")
            .databaseUser("app")
            .databasePassword(mariadbPassword.asSecret())
            .environment(Output.ofSecret("MARIADB_HOST=mariadb"))
            .build());

        var mongodb = new MongoDB("mongodb", MongoDBArgs.builder()
            .name("mvp-mongodb")
            .environmentId(environment.environmentId())
            .databaseUser("app")
            .databasePassword(mongodbPassword.asSecret())
            .environment(Output.ofSecret("MONGODB_HOST=mongodb"))
            .build());

        var redis = new Redis("redis", RedisArgs.builder()
            .name("mvp-redis")
            .environmentId(environment.environmentId())
            .databasePassword(redisPassword.asSecret())
            .environment(Output.ofSecret("REDIS_HOST=redis"))
            .build());

        var destination = new Destination("destination", DestinationArgs.builder()
            .name("mvp-destination")
            .provider("s3")
            .accessKey(destinationAccessKey)
            .secretAccessKey(destinationSecretAccessKey.asSecret())
            .bucket("dokploy-mvp-backups")
            .region("us-east-1")
            .endpoint("https://s3.us-east-1.amazonaws.com")
            .build());

        var postgresBackup = new Backup("postgresBackup", BackupArgs.builder()
            .schedule("0 0 * * *")
            .prefix("postgres-")
            .destinationId(destination.destinationId())
            .database("app")
            .postgresId(postgres.postgresId())
            .build());

        var applicationVolumeBackup = new VolumeBackup("applicationVolumeBackup", VolumeBackupArgs.builder()
            .name("mvp-application-volume-backup")
            .volumeName("mvp-application-data")
            .prefix("application-")
            .destinationId(destination.destinationId())
            .cronExpression("0 0 * * *")
            .applicationId(application.applicationId())
            .build());

        var composeVolumeBackup = new VolumeBackup("composeVolumeBackup", VolumeBackupArgs.builder()
            .name("mvp-compose-volume-backup")
            .volumeName("mvp-compose-data")
            .prefix("compose-")
            .destinationId(destination.destinationId())
            .cronExpression("0 0 * * *")
            .composeId(compose.composeId())
            .serviceName("web")
            .build());

        var applicationDomain = new Domain("applicationDomain", DomainArgs.builder()
            .applicationId(application.applicationId())
            .host(appHost)
            .port(80)
            .https(true)
            .certificateType("none")
            .enabled(true)
            .build());

        var composeDomain = new Domain("composeDomain", DomainArgs.builder()
            .composeId(compose.composeId())
            .serviceName("web")
            .host(composeHost)
            .port(80)
            .https(true)
            .certificateType("none")
            .enabled(true)
            .build());

        ctx.export("gitlabIntegration", gitlabIntegration);
        ctx.export("gitlabProject", gitlabProject);
        ctx.export("gitlabOwner", gitlabOwner);
        ctx.export("gitlabNamespace", gitlabNamespace);
        ctx.export("gitlabRepository", gitlabRepository);
        ctx.export("gitBranch", gitBranch);
    }
}
