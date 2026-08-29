import pulumi
import pulumi_dokploy as dokploy

config = pulumi.Config()
dokploy_endpoint = config.require_object("dokploy:endpoint")
dokploy_api_key = config.require_object("dokploy:apiKey")
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
redis_password = config.get_secret("redisPassword")
if redis_password is None:
    redis_password = "replace-with-a-redis-password"
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
redis = dokploy.Redis("redis",
    name="mvp-redis",
    environment_id=environment.environment_id,
    database_password=pulumi.Output.secret(redis_password),
    environment=pulumi.Output.secret("REDIS_HOST=redis"))
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
