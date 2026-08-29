import * as pulumi from "@pulumi/pulumi";
import * as dokploy from "@dimeskigj/pulumi-dokploy";

const config = new pulumi.Config();
const dokployEndpoint = config.require("dokploy:endpoint");
const dokployApiKey = config.requireSecret("dokploy:apiKey");
const appHost = config.get("appHost") || "app.example.invalid";
const composeHost = config.get("composeHost") || "compose.example.invalid";
const gitlabIntegrationConfig = config.get("gitlabIntegration") || "gitlab-integration-id";
const gitlabProjectConfig = config.getNumber("gitlabProject") || 42;
const gitlabOwnerConfig = config.get("gitlabOwner") || "example";
const gitlabNamespaceConfig = config.get("gitlabNamespace") || "platform";
const gitlabRepositoryConfig = config.get("gitlabRepository") || "application";
const gitBranchConfig = config.get("gitBranch") || "main";
const registryPassword = config.getSecret("registryPassword") || pulumi.secret("replace-with-a-registry-password");
const databasePassword = config.getSecret("databasePassword") || pulumi.secret("replace-with-a-database-password");
const redisPassword = config.getSecret("redisPassword") || pulumi.secret("replace-with-a-redis-password");
const projectResource = new dokploy.Project("project", {
    name: "dokploy-mvp",
    description: "Canonical Dokploy provider example",
});
const environment = new dokploy.Environment("environment", {
    projectId: projectResource.projectId,
    name: "staging",
    description: "Additional non-production environment",
});
const application = new dokploy.Application("application", {
    name: "mvp-application",
    environmentId: projectResource.defaultEnvironmentId,
    source: {
        type: "docker",
        docker: {
            image: "nginx:1.27",
            username: "example",
            password: pulumi.secret(registryPassword),
        },
    },
    environment: pulumi.secret(`APP_HOST=${appHost}`),
    createEnvFile: true,
});
const compose = new dokploy.Compose("compose", {
    name: "mvp-compose",
    environmentId: projectResource.defaultEnvironmentId,
    source: {
        type: "raw",
        raw: {
            composeFile: `services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
`,
        },
    },
    environment: pulumi.secret(`COMPOSE_HOST=${composeHost}`),
    createEnvFile: true,
});
const postgres = new dokploy.Postgres("postgres", {
    name: "mvp-postgres",
    environmentId: environment.environmentId,
    databaseName: "app",
    databaseUser: "app",
    databasePassword: pulumi.secret(databasePassword),
    environment: pulumi.secret("POSTGRES_HOST=postgres"),
});
const redis = new dokploy.Redis("redis", {
    name: "mvp-redis",
    environmentId: environment.environmentId,
    databasePassword: pulumi.secret(redisPassword),
    environment: pulumi.secret("REDIS_HOST=redis"),
});
const applicationDomain = new dokploy.Domain("applicationDomain", {
    applicationId: application.applicationId,
    host: appHost,
    port: 80,
    https: true,
    certificateType: "none",
    enabled: true,
});
const composeDomain = new dokploy.Domain("composeDomain", {
    composeId: compose.composeId,
    serviceName: "web",
    host: composeHost,
    port: 80,
    https: true,
    certificateType: "none",
    enabled: true,
});
export const gitlabIntegration = gitlabIntegrationConfig;
export const gitlabProject = gitlabProjectConfig;
export const gitlabOwner = gitlabOwnerConfig;
export const gitlabNamespace = gitlabNamespaceConfig;
export const gitlabRepository = gitlabRepositoryConfig;
export const gitBranch = gitBranchConfig;
