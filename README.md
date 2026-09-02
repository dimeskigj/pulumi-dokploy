# Pulumi Dokploy Provider

[![Build Status](https://github.com/dimeskigj/pulumi-dokploy/actions/workflows/build.yml/badge.svg)](https://github.com/dimeskigj/pulumi-dokploy/actions/workflows/build.yml)

The Dokploy provider manages self-hosted Dokploy projects and deployments with Pulumi.

**[Read the documentation](https://dimeskigj.github.io/pulumi-dokploy/)**

## Install

The provider is built and tested with Go 1.25.13, including the security-fixed
toolchain used by the release workflows.

Install the package for your Pulumi language:

| Language | Package |
| --- | --- |
| Node.js | `@dimeskigj/pulumi-dokploy` |
| Python | `pulumi_dokploy` |
| Go | `github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy` |
| .NET | `Pulumi.Dokploy` |
| Java | `net.dimeski.pulumi.dokploy` |
| YAML | `pulumi package add github.com/dimeskigj/pulumi-dokploy dokploy` |

The provider is also available from the Pulumi Registry once a release is published.

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

The provider exposes eighteen resources: `dokploy:index:Project`, `dokploy:index:Environment`,
`dokploy:index:Application`, `dokploy:index:Compose`, `dokploy:index:Postgres`,
`dokploy:index:MySQL`, `dokploy:index:MariaDB`, `dokploy:index:MongoDB`,
`dokploy:index:Redis`, `dokploy:index:Domain`, `dokploy:index:Destination`,
`dokploy:index:Backup`, `dokploy:index:VolumeBackup`, `dokploy:index:SSHKey`,
`dokploy:index:Registry`, `dokploy:index:Tag`, `dokploy:index:ProjectTag`, and
`dokploy:index:Mount`.

`Backup` schedules database backups (Postgres, MySQL, MariaDB, or MongoDB) to a `Destination`.
`VolumeBackup` schedules Docker volume backups for an `Application` or `Compose` service to a
`Destination`. Both require a `Destination` (an S3-compatible storage target) to exist first.

Project owns the default environment. Create explicit `Environment` resources for additional
environments and use their IDs from dependent resources. Applications and Compose stacks can
use Git, Docker, raw Compose, or private GitLab sources. A private GitLab reference records the
integration/project/owner/namespace/repository/branch details. The referenced GitLab integration is not managed
by this provider. SSH key references are likewise passed through and not managed.

Source type changes replace that resource rather than attempting an
in-place conversion. Create and update operations wait for Dokploy deployment completion;
deployment errors preserve partial state so the failed resource can be inspected and repaired.
Compose volumes are preserved on destroy by default. Set `deleteVolumesOnDestroy` only when
those volumes should be deleted.

Database passwords, environment values, application build arguments/build secrets, and nested
Docker credentials are secret inputs. Keep them secret in configuration and never log them.

SSH keys, container registries, reusable tags, project-tag associations, and workload mounts are
managed resources. Use an `SSHKey` output as the `sshKeyId` of a generic Git Application, and a
`Registry` output as an Application's `registryId` or `buildRegistryId`. Mounts support `bind`,
`volume`, and `file` targets; automatic mount redeployment occurs after a change. Registry
credentials, SSH private keys, and file contents are secret inputs; registries are tested before
create and credential-affecting updates through `testRegistry`. Supply `registryPassword` and `sshPrivateKey` through
secret configuration. MongoDB and LibSQL are documented exclusions for mounts.

See the [Get Started](https://dimeskigj.github.io/pulumi-dokploy/getting-started/installation/),
[Resources](https://dimeskigj.github.io/pulumi-dokploy/reference/), and
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
```

## Development

Use the pinned tools from `mise`:

```bash
mise install
make lint
make test
make build
make codegen
make check_openapi
make ci-mgmt
```

Generated SDKs and CI workflows must be regenerated rather than hand-edited. See
`CONTRIBUTING.md` for the complete workflow.
