---
title: Installation and configuration
description: Install and configure the community-maintained Pulumi Dokploy provider.
---

# Installation and configuration

The provider's package metadata points Pulumi at its GitHub release artifacts.
Pulumi can acquire the provider plugin automatically when a package is added or
used; no separate plugin download is normally required.

Install the SDK for your language:

```bash
npm install @dimeskigj/pulumi-dokploy
pip install pulumi_dokploy
go get github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy
dotnet add package Dimeskigj.Pulumi.Dokploy
mvn dependency:get -Dartifact=net.dimeski.pulumi:dokploy
pulumi package add github.com/dimeskigj/pulumi-dokploy dokploy
```

The last command adds the provider to a YAML program. The other commands install
the TypeScript, Python, Go, .NET, and Java SDKs respectively.

## Configuration

Pulumi configuration keys use the `dokploy` namespace:

```bash
pulumi config set dokploy:endpoint https://dokploy.example.com
pulumi config set --secret dokploy:apiKey your-api-key
```

`dokploy:endpoint` is the URL of the Dokploy instance. `dokploy:apiKey` is a
secret and should always be stored with `--secret`; Pulumi encrypts secret
configuration in the stack state. Do not commit API keys, put them in source
code, or print them in logs.

As an alternative, set `DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY` in the
environment running Pulumi. Environment variables are useful in CI when loaded
from the CI system's secret store. Keep both values out of shell history and
logs.

This package is community-maintained; neither Dokploy nor Pulumi maintains it.
