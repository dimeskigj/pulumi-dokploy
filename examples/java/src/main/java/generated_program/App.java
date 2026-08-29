package generated_program;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.core.Output;
import dev.codechem.pulumi.dokploy.Project;
import dev.codechem.pulumi.dokploy.ProjectArgs;
import dev.codechem.pulumi.dokploy.Environment;
import dev.codechem.pulumi.dokploy.EnvironmentArgs;
import dev.codechem.pulumi.dokploy.Application;
import dev.codechem.pulumi.dokploy.ApplicationArgs;
import dev.codechem.pulumi.dokploy.inputs.ApplicationSourceArgs;
import dev.codechem.pulumi.dokploy.inputs.DockerSourceArgs;
import dev.codechem.pulumi.dokploy.Compose;
import dev.codechem.pulumi.dokploy.ComposeArgs;
import dev.codechem.pulumi.dokploy.inputs.ComposeSourceArgs;
import dev.codechem.pulumi.dokploy.inputs.RawComposeSourceArgs;
import dev.codechem.pulumi.dokploy.Postgres;
import dev.codechem.pulumi.dokploy.PostgresArgs;
import dev.codechem.pulumi.dokploy.Redis;
import dev.codechem.pulumi.dokploy.RedisArgs;
import dev.codechem.pulumi.dokploy.Domain;
import dev.codechem.pulumi.dokploy.DomainArgs;
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
        final var registryPassword = config.getSecret("registryPassword").applyValue(v -> v.orElse("replace-with-a-registry-password"));
        final var databasePassword = config.getSecret("databasePassword").applyValue(v -> v.orElse("replace-with-a-database-password"));
        final var redisPassword = config.getSecret("redisPassword").applyValue(v -> v.orElse("replace-with-a-redis-password"));
        var projectResource = new Project("projectResource", ProjectArgs.builder()
            .name("dokploy-mvp")
            .description("Canonical Dokploy provider example")
            .build());

        var environment = new Environment("environment", EnvironmentArgs.builder()
            .projectId(projectResource.projectId())
            .name("staging")
            .description("Additional non-production environment")
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

        var postgres = new Postgres("postgres", PostgresArgs.builder()
            .name("mvp-postgres")
            .environmentId(environment.environmentId())
            .databaseName("app")
            .databaseUser("app")
            .databasePassword(databasePassword.asSecret())
            .environment(Output.ofSecret("POSTGRES_HOST=postgres"))
            .build());

        var redis = new Redis("redis", RedisArgs.builder()
            .name("mvp-redis")
            .environmentId(environment.environmentId())
            .databasePassword(redisPassword.asSecret())
            .environment(Output.ofSecret("REDIS_HOST=redis"))
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
