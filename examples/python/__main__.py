import pulumi
import pulumi_dokploy as dokploy

config = pulumi.Config()
dokploy_endpoint = config.require("dokploy:endpoint")
dokploy_api_key = config.require_secret("dokploy:apiKey")
app_host = config.get("appHost")
if app_host is None:
    app_host = "app.example.invalid"
compose_host = config.get("composeHost")
if compose_host is None:
    compose_host = "compose.example.invalid"
gitlab_integration = config.get("gitlabIntegration")
if gitlab_integration is None:
    gitlab_integration = "gitlab-integration-id"
gitlab_project = config.get_int("gitlabProject")
if gitlab_project is None:
    gitlab_project = 42
gitlab_owner = config.get("gitlabOwner")
if gitlab_owner is None:
    gitlab_owner = "example"
gitlab_namespace = config.get("gitlabNamespace")
if gitlab_namespace is None:
    gitlab_namespace = "platform"
gitlab_repository = config.get("gitlabRepository")
if gitlab_repository is None:
    gitlab_repository = "application"
git_branch = config.get("gitBranch")
if git_branch is None:
    git_branch = "main"
registry_password = config.get_secret("registryPassword")
if registry_password is None:
    registry_password = "replace-with-a-registry-password"
database_password = config.get_secret("databasePassword")
if database_password is None:
    database_password = "replace-with-a-database-password"
mysql_password = config.get_secret("mysqlPassword")
if mysql_password is None:
    mysql_password = "replace-with-a-mysql-password"
mariadb_password = config.get_secret("mariadbPassword")
if mariadb_password is None:
    mariadb_password = "replace-with-a-mariadb-password"
mongodb_password = config.get_secret("mongodbPassword")
if mongodb_password is None:
    mongodb_password = "replace-with-a-mongodb-password"
redis_password = config.get_secret("redisPassword")
if redis_password is None:
    redis_password = "replace-with-a-redis-password"
destination_access_key = config.get("destinationAccessKey")
if destination_access_key is None:
    destination_access_key = "replace-with-a-destination-access-key"
destination_secret_access_key = config.get_secret("destinationSecretAccessKey")
if destination_secret_access_key is None:
    destination_secret_access_key = "replace-with-a-destination-secret-access-key"
project_resource = dokploy.Project("project",
    name="dokploy-mvp",
    description="Canonical Dokploy provider example")
environment = dokploy.Environment("environment",
    project_id=project_resource.project_id,
    name="staging",
    description="Additional non-production environment")
application = dokploy.Application("application",
    name="mvp-application",
    environment_id=project_resource.default_environment_id,
    source={
        "type": "docker",
        "docker": {
            "image": "nginx:1.27",
            "username": "example",
            "password": pulumi.Output.secret(registry_password),
        },
    },
    environment=pulumi.Output.secret(f"APP_HOST={app_host}"),
    create_env_file=True)
compose = dokploy.Compose("compose",
    name="mvp-compose",
    environment_id=project_resource.default_environment_id,
    source={
        "type": "raw",
        "raw": {
            "compose_file": """services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
""",
        },
    },
    environment=pulumi.Output.secret(f"COMPOSE_HOST={compose_host}"),
    create_env_file=True)
postgres = dokploy.Postgres("postgres",
    name="mvp-postgres",
    environment_id=environment.environment_id,
    database_name="app",
    database_user="app",
    database_password=pulumi.Output.secret(database_password),
    environment=pulumi.Output.secret("POSTGRES_HOST=postgres"))
mysql = dokploy.MySQL("mysql",
    name="mvp-mysql",
    environment_id=environment.environment_id,
    database_name="app",
    database_user="app",
    database_password=pulumi.Output.secret(mysql_password),
    environment=pulumi.Output.secret("MYSQL_HOST=mysql"))
mariadb = dokploy.MariaDB("mariadb",
    name="mvp-mariadb",
    environment_id=environment.environment_id,
    database_name="app",
    database_user="app",
    database_password=pulumi.Output.secret(mariadb_password),
    environment=pulumi.Output.secret("MARIADB_HOST=mariadb"))
mongodb = dokploy.MongoDB("mongodb",
    name="mvp-mongodb",
    environment_id=environment.environment_id,
    database_user="app",
    database_password=pulumi.Output.secret(mongodb_password),
    environment=pulumi.Output.secret("MONGODB_HOST=mongodb"))
redis = dokploy.Redis("redis",
    name="mvp-redis",
    environment_id=environment.environment_id,
    database_password=pulumi.Output.secret(redis_password),
    environment=pulumi.Output.secret("REDIS_HOST=redis"))
destination = dokploy.Destination("destination",
    name="mvp-destination",
    provider="s3",
    access_key=destination_access_key,
    secret_access_key=pulumi.Output.secret(destination_secret_access_key),
    bucket="dokploy-mvp-backups",
    region="us-east-1",
    endpoint="https://s3.us-east-1.amazonaws.com")
postgres_backup = dokploy.Backup("postgresBackup",
    schedule="0 0 * * *",
    prefix="postgres-",
    destination_id=destination.destination_id,
    database="app",
    postgres_id=postgres.postgres_id)
application_volume_backup = dokploy.VolumeBackup("applicationVolumeBackup",
    name="mvp-application-volume-backup",
    volume_name="mvp-application-data",
    prefix="application-",
    destination_id=destination.destination_id,
    cron_expression="0 0 * * *",
    application_id=application.application_id)
compose_volume_backup = dokploy.VolumeBackup("composeVolumeBackup",
    name="mvp-compose-volume-backup",
    volume_name="mvp-compose-data",
    prefix="compose-",
    destination_id=destination.destination_id,
    cron_expression="0 0 * * *",
    compose_id=compose.compose_id,
    service_name="web")
application_domain = dokploy.Domain("applicationDomain",
    application_id=application.application_id,
    host=app_host,
    port=80,
    https=True,
    certificate_type="none",
    enabled=True)
compose_domain = dokploy.Domain("composeDomain",
    compose_id=compose.compose_id,
    service_name="web",
    host=compose_host,
    port=80,
    https=True,
    certificate_type="none",
    enabled=True)
pulumi.export("gitlabIntegration", gitlab_integration)
pulumi.export("gitlabProject", gitlab_project)
pulumi.export("gitlabOwner", gitlab_owner)
pulumi.export("gitlabNamespace", gitlab_namespace)
pulumi.export("gitlabRepository", gitlab_repository)
pulumi.export("gitBranch", git_branch)
