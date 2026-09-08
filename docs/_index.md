---
title: Dokploy
description: Pulumi provider for managing Dokploy resources.
---

# Dokploy

The Dokploy provider lets you manage Dokploy projects, applications, databases,
domains, backups, and related resources with Pulumi. This is a
**community-maintained** provider; neither Dokploy nor Pulumi maintains this
package.

## Installation

Install the package for your language using the [installation and configuration
guide](./installation-configuration/). Pulumi can acquire the provider plugin
automatically from its GitHub package metadata when your program first uses it.

## Configuration

Set `dokploy:endpoint` to your Dokploy instance URL and set the secret
`dokploy:apiKey` to an API key. These values can also be supplied with the
`DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY` environment variables.

## Example

```typescript
import * as pulumi from "@pulumi/pulumi";
import * as dokploy from "@dimeskigj/pulumi-dokploy";

const config = new pulumi.Config("dokploy");
const endpoint = config.require("endpoint");
const apiKey = config.requireSecret("apiKey");

const project = new dokploy.Project("example", {
  name: "example",
  description: "Managed by Pulumi",
});

export const projectId = project.id;
export { endpoint, apiKey };
```
