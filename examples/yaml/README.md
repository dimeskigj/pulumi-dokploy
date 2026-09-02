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

## SSH keys, registries, tags, and mounts

Sensitive values are supplied through Pulumi configuration and remain secret:

```yaml
configuration:
  dokploy:registryPassword:
    type: string
    secret: true
  dokploy:sshPrivateKey:
    type: string
    secret: true
```

The complete example provisions an `SSHKey` and passes its `sshKeyId` to a
generic Git Application, tests and creates a `Registry`, and connects a `Tag`
to the Project with `ProjectTag`. It also demonstrates bind, volume, and file
`Mount` resources. A mount change automatically redeploys its workload; mount
file content and registry credentials are write-only secrets.

MongoDB and LibSQL do not support mounts in this provider. Registry testing is
performed before create and before credential-affecting updates.

Resources can be imported with their Dokploy IDs (ProjectTag uses
`project-id/tag-id`), for example:

```text
pulumi import dokploy:index:SSHKey key <ssh-key-id>
pulumi import dokploy:index:Registry registry <registry-id>
pulumi import dokploy:index:Tag tag <tag-id>
pulumi import dokploy:index:ProjectTag projectTag <project-id>/<tag-id>
pulumi import dokploy:index:Mount mount <mount-id>
```
