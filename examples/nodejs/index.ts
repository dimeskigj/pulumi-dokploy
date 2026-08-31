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
const mysqlPassword = config.getSecret("mysqlPassword") || pulumi.secret("replace-with-a-mysql-password");
const mariadbPassword = config.getSecret("mariadbPassword") || pulumi.secret("replace-with-a-mariadb-password");
const mongodbPassword = config.getSecret("mongodbPassword") || pulumi.secret("replace-with-a-mongodb-password");
const redisPassword = config.getSecret("redisPassword") || pulumi.secret("replace-with-a-redis-password");
const destinationAccessKey = config.get("destinationAccessKey") || "replace-with-a-destination-access-key";
const destinationSecretAccessKey = config.getSecret("destinationSecretAccessKey") || pulumi.secret("replace-with-a-destination-secret-access-key");
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
const mysql = new dokploy.MySQL("mysql", {
    name: "mvp-mysql",
    environmentId: environment.environmentId,
    databaseName: "app",
    databaseUser: "app",
    databasePassword: pulumi.secret(mysqlPassword),
    environment: pulumi.secret("MYSQL_HOST=mysql"),
});
const mariadb = new dokploy.MariaDB("mariadb", {
    name: "mvp-mariadb",
    environmentId: environment.environmentId,
    databaseName: "app",
    databaseUser: "app",
    databasePassword: pulumi.secret(mariadbPassword),
    environment: pulumi.secret("MARIADB_HOST=mariadb"),
});
const mongodb = new dokploy.MongoDB("mongodb", {
    name: "mvp-mongodb",
    environmentId: environment.environmentId,
    databaseUser: "app",
    databasePassword: pulumi.secret(mongodbPassword),
    environment: pulumi.secret("MONGODB_HOST=mongodb"),
});
const redis = new dokploy.Redis("redis", {
    name: "mvp-redis",
    environmentId: environment.environmentId,
    databasePassword: pulumi.secret(redisPassword),
    environment: pulumi.secret("REDIS_HOST=redis"),
});
const destination = new dokploy.Destination("destination", {
    name: "mvp-destination",
    provider: "s3",
    accessKey: destinationAccessKey,
    secretAccessKey: pulumi.secret(destinationSecretAccessKey),
    bucket: "dokploy-mvp-backups",
    region: "us-east-1",
    endpoint: "https://s3.us-east-1.amazonaws.com",
});
const postgresBackup = new dokploy.Backup("postgresBackup", {
    schedule: "0 0 * * *",
    prefix: "postgres-",
    destinationId: destination.destinationId,
    database: "app",
    postgresId: postgres.postgresId,
});
const applicationVolumeBackup = new dokploy.VolumeBackup("applicationVolumeBackup", {
    name: "mvp-application-volume-backup",
    volumeName: "mvp-application-data",
    prefix: "application-",
    destinationId: destination.destinationId,
    cronExpression: "0 0 * * *",
    applicationId: application.applicationId,
});
const composeVolumeBackup = new dokploy.VolumeBackup("composeVolumeBackup", {
    name: "mvp-compose-volume-backup",
    volumeName: "mvp-compose-data",
    prefix: "compose-",
    destinationId: destination.destinationId,
    cronExpression: "0 0 * * *",
    composeId: compose.composeId,
    serviceName: "web",
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
