# Dokploy Pulumi Provider MVP Design

## Summary

Build a Registry-ready native Pulumi provider for Dokploy. The MVP manages the
smallest complete platform slice that can deploy applications, Compose stacks,
Postgres, and Redis behind domains:

- `Project`
- `Environment`
- `Application`
- `Compose`
- `Postgres`
- `Redis`
- `Domain`

The provider is implemented in Go with `pulumi-go-provider/infer`. It uses a Go
API client generated from a normalized subset of Dokploy's OpenAPI document and
adds hand-authored Pulumi lifecycle semantics around that client.

## Goals

- Make a successful `pulumi up` produce running Dokploy workloads, not only
  persisted Dokploy records.
- Support create, read, update, delete, import, refresh, previews, drift
  detection, secrets, and useful diffs for every MVP resource.
- Support Docker image, public Git, and private GitLab application sources.
- Support raw, generic Git, and private GitLab Compose sources.
- Generate idiomatic Pulumi SDKs and documentation for Registry-required
  languages.
- Keep the API contract reproducible and make Dokploy API drift visible.
- Run comprehensive tests without requiring a live Dokploy instance by
  default.

## Non-Goals

- Expose all Dokploy API operations in the MVP.
- Manage GitLab integrations, SSH keys, or their credentials. MVP resources
  reference integrations and keys that already exist in Dokploy.
- Support GitHub, Bitbucket, or Gitea Compose sources.
- Support GitHub, Bitbucket, Gitea, or authenticated generic Git application
  sources.
- Manage MySQL, MariaDB, MongoDB, LibSQL, backups, mounts, ports, schedules,
  registries, clusters, servers, organizations, notifications, or advanced
  Swarm settings.
- Guarantee Pulumi Registry acceptance or provision package-feed credentials.

## Research Findings

Dokploy exposes authenticated operations under `/api` using the `x-api-key`
header. Each instance serves Swagger at `/swagger` and can return a
version-matched OpenAPI document through `settings.getOpenApiDocument`, subject
to user permissions.

Dokploy also commits an OpenAPI 3.1 document at the root of its upstream
repository. At the time of this design, the document contains 554 paths. It is
useful for operation names, request shapes, validation constraints, security,
and error responses. Most successful responses, however, are emitted as empty
objects even when the implementation returns resource data. It is therefore
not sufficient as the sole source for a Pulumi provider client or state model.

Pulumi recommends implementing public SaaS providers in Go with the Pulumi Go
Provider SDK. Inferred schemas remove the need to maintain a Pulumi schema by
hand while still enabling multi-language SDK and documentation generation.

## Architecture

### Provider

The package is a native Go provider built with `pulumi-go-provider/infer`. The
provider exports seven custom resources and centralizes configuration, client
creation, logging, retry policy, and error classification.

Provider configuration contains:

- `endpoint`: Dokploy instance URL. Defaults from `DOKPLOY_ENDPOINT`.
- `apiKey`: Dokploy API key. It is secret and defaults from
  `DOKPLOY_API_KEY`.

The client normalizes `endpoint` so requests contain exactly one `/api` suffix,
sets `x-api-key`, uses bounded timeouts, respects context cancellation, and
never logs credentials or sensitive request bodies.

### Repository Boundaries

- `provider/`: entrypoint, configuration, common provider behavior, and the
  seven resource implementations.
- `internal/client/`: generated MVP API client and a small adapter for response
  decoding, retries, and error classification.
- `openapi/`: pinned upstream contract, source metadata, endpoint selection,
  success-response corrections, and deterministic generation scripts.
- `sdk/`: generated language SDKs.
- `examples/`: deployable examples used for documentation and validation.
- Registry and release metadata at repository-standard locations.

Resource files remain focused on Pulumi lifecycle behavior. HTTP encoding and
Dokploy response details remain inside the client package.

## OpenAPI Strategy

The repository checks in Dokploy's upstream `openapi.json` together with the
source commit or tag and a checksum. A deterministic generation pipeline:

1. Selects only operations required by the MVP resources.
2. Applies checked-in corrections for successful response schemas that
   upstream represents as empty objects.
3. Generates the Go API client from the normalized subset.
4. Formats generated code and fails CI when regeneration changes tracked
   files.

Corrections describe observed response contracts only. They do not change
request semantics. Each correction is covered by an HTTP fixture test and can
be compared with the corresponding Dokploy router implementation.

Schema updates are explicit. A maintainer runs one command to fetch or copy a
new pinned upstream document, regenerate the normalized subset and client, and
review endpoint and model drift before adopting the new Dokploy version.

The complete upstream API is not exposed through the Pulumi SDK.

## Resource Model

### Project

Inputs:

- `name`
- `description`

Outputs include `projectId` and `defaultEnvironmentId`.

Dokploy creates a mandatory default `production` environment with each project.
The `Project` resource owns that environment indirectly and exposes its ID. The
default environment is not represented by a separate `Environment` resource
because Dokploy does not permit deleting or renaming it.

Name and description update in place. Delete removes the project and its
Dokploy-owned descendants.

### Environment

Inputs:

- `projectId`
- `name`
- `description`

This resource manages additional, non-default environments. `projectId` is
replacement-only. Name and description update in place. Check rejects the
reserved name `production`.

### Application

Common inputs include:

- `name`
- `appName`
- `description`
- `environmentId`
- `serverId`
- `source`
- environment text
- build arguments
- build secrets
- create-env-file behavior

`source` is a discriminated object with exactly one of these variants:

- `docker`: image, optional registry URL, optional username, and secret
  password.
- `git`: public repository URL, branch, build path, and build configuration.
- `gitlab`: existing Dokploy `gitlabId`, GitLab project ID, owner or namespace,
  repository, branch, build path, and build configuration.

Git and GitLab builds initially support `nixpacks` and `dockerfile`. Conditional
validation requires Dockerfile-specific fields only for Dockerfile builds.

Creation establishes the application, configures its source, build, and
environment, deploys it, and waits for completion. Metadata updates do not
redeploy. Source, build, image, or environment changes redeploy.

Source-type changes update the existing resource only if contract tests prove
Dokploy reliably clears stale source fields. Otherwise `source.type` is marked
replacement-only before the first release.

### Compose

Common inputs include:

- `name`
- `appName`
- `description`
- `environmentId`
- `serverId`
- `composeType`
- `source`
- environment text
- create-env-file behavior
- `deleteVolumesOnDestroy`, defaulting to `false`

`source` is a discriminated object with exactly one of these variants:

- `raw`: inline Compose document.
- `git`: repository URL, branch, Compose file path, optional existing Dokploy
  SSH-key ID, watch paths, and submodule setting.
- `gitlab`: existing Dokploy `gitlabId`, GitLab project ID, owner or namespace,
  repository, branch, Compose file path, watch paths, and submodule setting.

Creation establishes the Compose record, applies source-specific fields,
fetches repository content when required, deploys, and waits for completion.
Runtime-affecting source, Compose, or environment changes fetch when needed and
redeploy. Metadata-only changes do not redeploy.

Source-type changes follow the same contract-test rule as `Application`.

### Postgres

Inputs include:

- `name`
- `appName`
- `description`
- `environmentId`
- `serverId`
- database name
- database user
- secret database password
- Docker image
- environment text
- optional external port

Creation configures and deploys the database, then waits for completion.
Runtime-affecting updates redeploy or reload as required by the Dokploy API.
Parent environment and server placement are replacement-only in the MVP.

### Redis

Inputs include:

- `name`
- `appName`
- `description`
- `environmentId`
- `serverId`
- secret database password
- Docker image
- environment text
- optional external port

Creation configures and deploys Redis, then waits for completion.
Runtime-affecting updates redeploy or reload as required by the Dokploy API.
Parent environment and server placement are replacement-only in the MVP.

### Domain

Inputs include:

- exactly one target: `applicationId`, or `composeId` with `serviceName`
- `host`
- `path`
- `internalPath`
- `port`
- HTTPS setting
- certificate mode and custom resolver when applicable
- strip-path setting
- enabled setting

Target changes are replacement-only because Dokploy's domain update operation
does not retarget a domain. Routing fields update in place.

## State And Secrets

Each resource uses its Dokploy ID as the Pulumi resource ID and outputs stable,
useful observed fields. Deployment status is an observed output, not desired
state, and volatile deployment timestamps or queue state are excluded from
diffs.

Registry passwords, database passwords, environment text, build arguments, and
build secrets are marked secret throughout configuration, generated SDKs, and
resource state. Read preserves prior secret values when Dokploy omits or redacts
them. The provider does not invent values for unavailable secrets during
import.

## Lifecycle

Preview validates inputs, applies defaults, and calculates diffs without
network mutations.

Create follows this sequence:

1. Validate discriminated inputs and conditional requirements.
2. Create the Dokploy record and retain its ID immediately.
3. Apply secondary source, build, environment, or port configuration.
4. Start deployment.
5. Poll the resource read operation until success or failure, respecting Pulumi
   cancellation and custom resource timeouts.

If configuration fails before deployment starts, the provider best-effort
removes the empty record. If deployment fails after a record exists, the
provider preserves the ID and observable state as a partial creation and
returns the deployment error.

Update calls only endpoints required by the detailed diff. Runtime-affecting
changes deploy and wait. Metadata-only changes do not deploy. Failed updates
leave the remote resource available for inspection and allow a later update or
destroy to retry safely.

Read fetches by Pulumi ID and normalizes remote data into provider state. A
missing remote object removes the resource from Pulumi state. Delete treats an
already absent object as success.

Import accepts a raw Dokploy ID. It populates all observable inputs and leaves
unavailable write-only secrets unknown. Imported resources can refresh
immediately and become fully managed when required secret inputs are supplied
in the Pulumi program.

## Errors And Retries

The client maps Dokploy error responses into concise errors containing the
operation, resource type or ID when known, API error code, and message.

- `400` and other validation failures are non-retryable.
- `401` and `403` identify authentication or permission failures and are
  non-retryable.
- `404` during read or delete means the resource is absent.
- `429` and transient `5xx` responses use bounded exponential backoff, honoring
  server retry hints when present.
- Deployment failure stops polling and includes the useful Dokploy failure
  message.

Retry loops and deployment polling respect context cancellation and Pulumi
resource timeouts. Logs redact API keys, passwords, environment values, build
arguments, build secrets, and registry credentials.

## Testing

Normal CI does not require a live Dokploy instance.

HTTP contract tests use `httptest` to verify methods, paths, query parameters,
request bodies, API-key handling, response decoding, errors, retries,
cancellation, and sensitive-value redaction for every generated operation used
by the provider.

Provider lifecycle tests use a scripted fake Dokploy server to cover:

- preview without mutation
- defaults and input validation
- replacement and in-place diffs
- create, configure, deploy, and poll sequences
- metadata-only updates
- runtime redeploys
- failed configuration and deployment
- refresh and drift
- imports and unavailable secrets
- secret propagation
- idempotent deletion
- timeout and cancellation
- every supported Application and Compose source variant

Schema tests generate all supported SDKs and documentation and compare tracked
outputs. Generation checks fail when committed OpenAPI-derived or Pulumi SDK
artifacts are stale.

Optional acceptance tests run only when `DOKPLOY_ENDPOINT` and
`DOKPLOY_API_KEY` are set. They create uniquely named resources and always
attempt cleanup. The supplied `dokploy.codechem.dev` instance may be used when
credentials are available, but live tests are not required by ordinary CI.

## Registry And Release Readiness

The MVP includes:

- Apache-2.0 licensing and contribution/security documentation
- Pulumi package, repository, publisher, and plugin metadata
- generated API documentation
- installation and provider configuration documentation
- deployable examples for supported Pulumi languages
- provider binaries for standard release platforms
- SDK generation and publication workflows for Registry-required ecosystems
- changelog and semantic version tooling
- dependency, license, and security scanning
- release checks that rebuild the provider, schema, SDKs, docs, and examples

The milestone is ready for Registry submission. Registry review, namespace
approval, and package-feed credentials are external release prerequisites.

## MVP Acceptance Criteria

A representative program can:

1. Configure the provider from Pulumi config or Dokploy environment variables.
2. Create a project and use its default environment, or create an additional
   environment.
3. Deploy an application from a Docker image, public Git repository, or private
   GitLab repository through an existing Dokploy integration.
4. Deploy a Compose stack from inline YAML, generic Git, or private GitLab.
5. Deploy Postgres and Redis with secrets preserved.
6. Attach domains to both an application and a Compose service.
7. Complete `pulumi preview`, `up`, refresh, import, update, and destroy with
   accurate diffs and no secret leakage.
8. Reach Dokploy's successful deployment state before `pulumi up` reports
   success for deployable resources.
9. Generate Registry-ready SDKs and documentation from the provider schema.

## Follow-On Work

Likely post-MVP increments are managed Git integrations and SSH keys, remaining
Git providers, additional application build modes, remaining database engines,
mounts and ports, backups, schedules, registries, server and cluster resources,
and broader Dokploy version compatibility testing. Each increment should retain
curated Pulumi lifecycle semantics rather than expose generic API calls.
