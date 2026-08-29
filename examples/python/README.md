# Dokploy MVP example source notes

The generated examples in each language are produced from this canonical YAML
program. These inactive alternatives document the supported source shapes:

## Git source alternative

```yaml
source:
  type: git
  git:
    url: https://github.com/example/application.git
    branch: main
    build:
      type: nixpacks
```

## GitLab source alternative

```yaml
source:
  type: gitlab
  gitlab:
    integrationId: ${gitlabIntegration}
    projectId: ${gitlabProject}
    owner: ${gitlabOwner}
    namespace: ${gitlabNamespace}
    repository: ${gitlabRepository}
    branch: ${gitBranch}
    build:
      type: nixpacks
```

The canonical program deliberately enables the Docker and raw Compose sources
instead; the alternatives above are documentation only.
