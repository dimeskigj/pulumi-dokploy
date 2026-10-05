# Pulumi Dokploy Provider

[![Build Status](https://github.com/dimeskigj/pulumi-dokploy/actions/workflows/build.yml/badge.svg)](https://github.com/dimeskigj/pulumi-dokploy/actions/workflows/build.yml)

The Dokploy provider manages self-hosted Dokploy projects and deployments with Pulumi.

**[Read the documentation](https://dimeskigj.github.io/pulumi-dokploy/)**

## Install

The provider is built and tested with Go 1.26.9, including the security-fixed
toolchain used by the release workflows.

Install the SDK package for your Pulumi language. Pulumi normally downloads the
matching provider plugin from GitHub Releases automatically when you use the package;
you do not need to install the plugin separately.

| Language | Package |
| --- | --- |
| Node.js | `@dimeskigj/pulumi-dokploy` |
| Python | `pulumi_dokploy` |
| Go | `github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy` |
| .NET | `Dimeskigj.Pulumi.Dokploy` |
| Java | `net.dimeski.pulumi.dokploy` |
| YAML | `pulumi package add github.com/dimeskigj/pulumi-dokploy dokploy` |

For Java, add the `net.dimeski.pulumi:dokploy` dependency at your chosen version
to `pom.xml`. See the [installation guide](https://dimeskigj.github.io/pulumi-dokploy/getting-started/installation/)
for language-specific commands.

## Configuration

Configure the Dokploy URL and API key in the Pulumi stack. The API key is always secret:

```bash
pulumi config set dokploy:endpoint https://dokploy.example.com
pulumi config set --secret dokploy:apiKey "$DOKPLOY_API_KEY"
```

The same values can be supplied with `DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY`. Do not put
API keys, database passwords, build secrets, or Docker registry passwords in source control;
use `pulumi config set --secret` (or secret outputs). Secret inputs and derived state remain
redacted in Pulumi diagnostics.

## Resources

Read-only ID-based lookups for projects, environments, workloads, databases, servers,
registries, and SSH keys return non-secret metadata without adopting or managing existing
objects. See the [lookup guide](https://dimeskigj.github.io/pulumi-dokploy/guides/lookups/).

The provider exposes these resources: `dokploy:index:Project`, `dokploy:index:Environment`,
`dokploy:index:Application`, `dokploy:index:Compose`, `dokploy:index:Postgres`,
`dokploy:index:MySQL`, `dokploy:index:MariaDB`, `dokploy:index:MongoDB`,
`dokploy:index:Redis`, `dokploy:index:Domain`, `dokploy:index:Destination`,
`dokploy:index:Backup`, `dokploy:index:VolumeBackup`, `dokploy:index:Schedule`, `dokploy:index:SSHKey`,
`dokploy:index:Registry`, `dokploy:index:Tag`, `dokploy:index:ProjectTag`,
`dokploy:index:Mount`, `dokploy:index:Port`, `dokploy:index:Notification`, and `dokploy:index:Server`.
The provider exposes twenty resources: `dokploy:index:Project`, `dokploy:index:Environment`,
`dokploy:index:Application`, `dokploy:index:Compose`, `dokploy:index:Postgres`,
`dokploy:index:MySQL`, `dokploy:index:MariaDB`, `dokploy:index:MongoDB`,
`dokploy:index:Redis`, `dokploy:index:Domain`, `dokploy:index:Destination`,
`dokploy:index:Backup`, `dokploy:index:VolumeBackup`, `dokploy:index:SSHKey`,
`dokploy:index:Registry`, `dokploy:index:Tag`, `dokploy:index:ProjectTag`, and
`dokploy:index:Mount`, `dokploy:index:Schedule`, and `dokploy:index:GitLabIntegration`.

`Backup` schedules database backups (Postgres, MySQL, MariaDB, or MongoDB) to a `Destination`.
`VolumeBackup` schedules Docker volume backups for an `Application` or `Compose` service to a
`Destination`. Both require a `Destination` (an S3-compatible storage target) to exist first.

Project owns the default environment. Create explicit `Environment` resources for additional
environments and use their IDs from dependent resources. Applications support Docker image sources
and Git, GitLab, GitHub, Gitea, and Bitbucket repository sources; Compose supports raw, Git, and
GitLab sources. Repository sources support all six builders: Dockerfile, Nixpacks, Heroku Buildpacks,
Paketo Buildpacks, Static, and Railpack. Builder-specific options are documented in the
[Applications guide](https://dimeskigj.github.io/pulumi-dokploy/guides/applications/).
Manage GitLab integration configuration with `GitLabIntegration` or continue using an external
relation ID; a referenced GitLab integration is not managed unless you declare that resource.
The provider does not perform OAuth: authorize in Dokploy before deploying GitLab-backed
workloads. See the [GitLab integrations guide](https://dimeskigj.github.io/pulumi-dokploy/guides/gitlab-integrations/).
SSH keys can be managed using the `SSHKey` resource.

Application `autoDeploy` is optional: omission leaves Dokploy's value unmanaged, while an explicit
boolean configures automatic triggers. This setting does not suppress the provider's explicit
deployment during application create or runtime updates; changing only `autoDeploy` is
configuration-only. Configuration-only live source coverage checks persistence rather than external
clone/build/deploy compatibility, and optional integration cases skip when protected prerequisites
are unavailable.

Source type changes replace that resource rather than attempting an
in-place conversion. Create and update operations for deploying workload resources
wait for Dokploy deployment completion; deployment errors preserve partial state so the failed resource can be inspected and repaired.
Compose volumes are preserved on destroy by default. Set `deleteVolumesOnDestroy` only when
those volumes should be deleted.

Database passwords, environment values, application build arguments/build secrets, and nested
Docker credentials are secret inputs. Keep them secret in configuration and never log them.
GitLab OAuth application secrets are also secret inputs; access and refresh tokens are never exported.

SSH keys, container registries, reusable tags, project-tag associations, and workload mounts are
managed resources. Use an `SSHKey` output as the `sshKeyId` of a generic Git Application, and a
`Registry` output as an Application's `registryId` or `buildRegistryId`. Mounts support `bind`,
`volume`, and `file` targets; automatic mount redeployment occurs after a change. Registry
credentials, SSH private keys, and file contents are secret inputs; registries are tested before
create and credential-affecting updates through `testRegistry`. Supply `registryPassword` and `sshPrivateKey` through
secret configuration. File mounts consume a dedicated `fileMountContent` secret configuration input;
the canonical example has only empty non-credential placeholders required by Pulumi YAML conversion.
MongoDB and LibSQL are documented exclusions for mounts.

`Port` maps an application's published and target ports. Create, update, and delete are
configuration-only and do not deploy or redeploy the application. See the
[Ports guide](https://dimeskigj.github.io/pulumi-dokploy/guides/ports/) for lifecycle and import details.

`Notification` manages one of twelve channel destinations and explicit event subscriptions;
all events default to false. Supply webhook URLs, tokens, endpoints, and custom headers
through Pulumi secret configuration. The provider creates with a temporary name and
disabled events, then verifies identity before applying final settings; if creation is
uncertain, investigate in Dokploy rather than retrying or deleting by name alone.
Refresh before repairing verified partial state. Changing channel replaces the resource;
the provider does not send automatic test messages. See the
[Notifications guide](https://dimeskigj.github.io/pulumi-dokploy/guides/notifications/).

See the [Get Started](https://dimeskigj.github.io/pulumi-dokploy/getting-started/installation/),
[Resources](https://dimeskigj.github.io/pulumi-dokploy/reference/project/), and
[Guides](https://dimeskigj.github.io/pulumi-dokploy/guides/applications/) pages for the full walkthrough.

## Import

Existing resources can be adopted with their Pulumi token and Dokploy ID:

```bash
pulumi import dokploy:index:Project existing p1
```

After import, review the generated state and supply any write-only secret inputs required for
future updates.

```bash
pulumi import dokploy:index:SSHKey key <ssh-key-id>
pulumi import dokploy:index:Registry registry <registry-id>
pulumi import dokploy:index:Tag tag <tag-id>
pulumi import dokploy:index:ProjectTag projectTag <project-id>/<tag-id>
pulumi import dokploy:index:Mount mount <mount-id>
pulumi import dokploy:index:Port mapping <port-id>
pulumi import dokploy:index:Notification notification <notification-id>
pulumi import dokploy:index:Server server <server-id>
```

An imported Notification may have required secrets omitted from Dokploy's response;
resupply them via secret configuration before update. Never use empty imported credentials.

Server import uses the Dokploy server record ID; it does not configure or bootstrap the remote host.
Server status is descriptive rather than a readiness guarantee. Deleting a Server removes its Dokploy
record and associated deployment history/logs, not the remote VM or installed software; Dokploy refuses
removal while workloads remain associated. See the [Servers guide](https://dimeskigj.github.io/pulumi-dokploy/guides/servers/)
for safe registration, cleanup opt-in, and workload-placement guidance.

## Development

Use the pinned tools from `mise`:

```bash
mise install
make lint
make test
make build
make codegen
make check_openapi
```

Generated SDKs must be regenerated rather than hand-edited. See
`CONTRIBUTING.md` for the complete workflow.
