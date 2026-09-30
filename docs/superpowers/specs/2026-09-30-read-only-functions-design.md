# Read-only Dokploy lookup functions

## Intent and approved constraints

Allow Pulumi programs to reference existing Dokploy infrastructure that is intentionally managed outside the current stack, without importing it or assuming lifecycle ownership.

The user approved:

- ID-only lookups, with no name discovery or scoped searches.
- Engine-specific database functions rather than a generic `getDatabase`.
- Non-secret metadata only.
- Dedicated typed function results, separate from managed-resource state.
- Direct read endpoint calls, including default-environment reads.
- OpenAPI source changes for `server.one`, generated SDKs, documentation, and mocked regression coverage.

Success means all twelve functions are available through the provider schema and all five generated SDKs, return only their documented metadata, and perform no writes or resource registration. Existing managed-resource contracts remain unchanged.

## Architecture and alternatives

Register inferred functions alongside resources in `provider/provider.go`, with tokens in the existing `dokploy:index` module. Each function has a small typed input and a dedicated typed result, uses `configuredClient`, and calls its corresponding authenticated GET endpoint.

Recommended approach: explicitly project API responses into allowlisted lookup result types. This provides useful SDK typing and makes the output security boundary visible in code and schema.

Rejected alternatives:

1. Reuse resource states and resource `Read` methods. Resource states contain secrets, source configuration, input defaults, and lifecycle restrictions. In particular, `Environment.Read` rejects default environments, which must remain readable by a lookup.
2. Return raw objects or generic maps. These weaken SDK typing and allow sensitive or newly added upstream fields to escape unnoticed.

Keep each function understandable independently. Shared helpers may handle ID validation, response identity checks, and safe diagnostics, but must not hide endpoint selection or output projection behind reflection. Do not refactor unrelated resource implementations.

## Function contracts

Inputs contain exactly one required string. IDs are opaque: validate that their trimmed form is nonempty, but send the original value unchanged. Do not echo supplied IDs in diagnostics.

| Function token suffix | Input/output ID field | Read operation |
| --- | --- | --- |
| `getProject` | `projectId` | `project.one` |
| `getEnvironment` | `environmentId` | `environment.one` |
| `getApplication` | `applicationId` | `application.one` |
| `getCompose` | `composeId` | `compose.one` |
| `getPostgres` | `postgresId` | `postgres.one` |
| `getMySQL` | `mysqlId` | `mysql.one` |
| `getMariaDB` | `mariadbId` | `mariadb.one` |
| `getMongoDB` | `mongoId` | `mongo.one` |
| `getRedis` | `redisId` | `redis.one` |
| `getServer` | `serverId` | `server.one` |
| `getRegistry` | `registryId` | `registry.one` |
| `getSSHKey` | `sshKeyId` | `sshKey.one` |

The names above are schema names. Language-specific casing follows Pulumi SDK generation conventions; no hand-written SDK naming overrides are required.

Every result requires its ID and `name`. A successful response must include a nonempty ID matching the requested ID and a present string name. Preserve the returned name verbatim, including an empty string if upstream supplies one. All additional fields below are optional and must be absent when upstream omits them or returns null. Preserve explicit empty strings, false booleans, and zero numbers; do not infer defaults from resource creation behavior.

| Result | Additional output fields |
| --- | --- |
| Project | `description`, `defaultEnvironmentId` |
| Environment | `description`, `projectId`, `isDefault` |
| Application | `description`, `appName`, `environmentId`, `serverId`, `status`, `registryId`, `buildRegistryId` |
| Compose | `description`, `appName`, `environmentId`, `serverId`, `status`, `composeType` |
| Postgres | `description`, `appName`, `environmentId`, `serverId`, `dockerImage`, `status`, `externalPort`, `databaseName`, `databaseUser` |
| MySQL | `description`, `appName`, `environmentId`, `serverId`, `dockerImage`, `status`, `externalPort`, `databaseName`, `databaseUser` |
| MariaDB | `description`, `appName`, `environmentId`, `serverId`, `dockerImage`, `status`, `externalPort`, `databaseName`, `databaseUser` |
| MongoDB | `description`, `appName`, `environmentId`, `serverId`, `dockerImage`, `status`, `externalPort`, `databaseUser`, `replicaSets` |
| Redis | `description`, `appName`, `environmentId`, `serverId`, `dockerImage`, `status`, `externalPort` |
| Server | `description`, `ipAddress`, `port`, `username`, `organizationId`, `sshKeyId`, `serverType`, `status` |
| Registry | `url`, `username`, `imagePrefix`, `serverId`, `registryType` |
| SSHKey | `description`, `publicKey`, `organizationId` |

All fields are strings except `isDefault` and `replicaSets` (booleans), and `externalPort` and server `port` (integers). Status and type outputs are strings, not restrictive enums, so new upstream values remain readable.

### Upstream field mapping

- Registry `registryName` maps to `name`, and `registryUrl` to `url`.
- Application and database `applicationStatus` map to `status`; Compose `composeStatus` and server `serverStatus` map to `status`.
- Database `dockerImage` maps to `dockerImage`. Support the existing correction model's legacy `image` spelling as a fallback only when `dockerImage` is absent/null; never substitute a provider default.
- `getEnvironment` reads the returned `isDefault` flag and accepts both true and false. If the flag is absent, leave it absent rather than inventing false. No standalone-resource restrictions apply.
- Project `defaultEnvironmentId` is returned only when supplied. Do not guess the default from a name, another ID field, or the first related environment.
- Return only flat fields in the table; do not traverse relations to populate missing metadata.

## Data flow and execution semantics

1. Pulumi invokes the function with its typed ID input and selected provider configuration.
2. Validate the ID before constructing/calling the HTTP client.
3. Use the existing configured client, authentication, cancellation, timeout, and bounded GET retry behavior.
4. Perform the single selected GET operation; retries may repeat this same GET. No list scans, follow-up discovery, credential tests, deployment polls, or mutation endpoints.
5. Validate success and response identity, then construct the allowlisted result.
6. Return it as an ordinary Pulumi function result, without creating a managed resource.

These functions may run during preview when their inputs are known. Preview still requires server connectivity and read permissions. Unknown-input and Output-form invoke behavior must follow the inference framework and generated SDK conventions, not custom resource dry-run logic. A lookup does not import, adopt, refresh, or delete the referenced object.

## Security and errors

Omit all passwords (including root/registry passwords), SSH private keys, environment variables, build arguments/secrets, access tokens, source objects, Compose file contents, commands, monitoring configuration, and nested API objects. Additional or future upstream fields must not automatically appear in outputs.

Public keys, login usernames, addresses, resource IDs, and ordinary metadata are intentionally available in results, but private URLs and resource IDs must not be printed in logs or diagnostics. Registry URLs containing URL user information, query strings, or fragments are omitted conservatively to avoid exposing embedded credentials or tokens. The provider does not claim to detect secrets deliberately stored inside arbitrary names or descriptions; document the distinction between metadata projection and arbitrary-content secret scanning.

Failure behavior:

- Missing/empty/whitespace-only input ID: a field-specific validation error, without making an HTTP request.
- HTTP 404 or an API error classified as `NOT_FOUND`: an explicit lookup-not-found error, never an empty result.
- Authentication, authorization, timeout, cancellation, network, or other API failures: return an error that retains a useful safe category and function/operation context.
- Null success object, missing/empty response ID, missing/null name, mismatched ID, invalid JSON, or incompatible field types: fail with a response-contract error.
- Optional metadata missing/null: success with that field absent.

Do not attach response bodies, server-provided free-form messages, decoder input excerpts, request URLs, API keys, or IDs to returned diagnostic text. Preserve context cancellation/deadline classification where feasible. Apply this at the lookup boundary rather than changing unrelated resource error contracts.

## OpenAPI and code generation

`openapi/upstream.json` is pinned by `openapi/source.json` to Dokploy commit `cebd3808565ea9bed0791961bc25c60513d94c5a`. Its `server.one` success schema is an empty object, so generating the endpoint without a source correction is insufficient.

Add only `server.one` to `openapi/operations.txt`, map its success response through `openapi/corrections.json`, and define a flat Server response schema for the allowlisted metadata. The pinned server router returns a server record plus relations; the pinned database schema confirms the field spellings in this design. Ignore relations and monitoring configuration even if present in JSON.

Add typed response metadata needed by the lookups to existing corrections: environment `isDefault`, Compose `composeStatus`, and database `dockerImage` / `applicationStatus`. Retain existing fields and legacy image support so resource reads are not inadvertently changed. Add null support where newly consumed optional metadata is nullable upstream. Test normalization and generated decoding behavior.

Regenerate `openapi/dokploy.json` and the generated client from their sources. Regenerate the provider schema and Go, Node.js, Python, .NET, and Java SDKs with the Makefile targets. Do not hand-edit generated output or add dependencies solely for these lookups.

## Documentation and examples

Give every function, argument, result, and result field a schema description. Replace the schema test's explicit assertion that functions are absent with assertions for the twelve documented function tokens and their contracts; retain the existing resource assertions.

Extend the existing website reference model and renderer to include function input/output tables. Generate one flat reference page per function, linked through the existing reference navigation mechanism, while preserving resource/configuration/type pages and deterministic generation. Function pages must clearly state that lookups are read-only and return non-secret metadata.

Add curated lookup guidance showing an externally managed project/environment referenced by ID, distinguish invokes from resource static `get`/import, describe preview connectivity and missing-object behavior, and explain the omitted sensitive fields. Use placeholders, never actual IDs, endpoints, or credentials. Include a compiled or mocked SDK usage example and ensure documentation tests cover generated function pages. Do not regenerate unrelated application examples unnecessarily or introduce a live lookup into routine example validation.

## Tests and validation

Use TDD during implementation. All behavior tests use local mock servers or mocked Pulumi runtime calls, never a real Dokploy endpoint.

Required regression coverage:

- Each function reaches its exact GET endpoint with the correct ID query parameter and returns the documented field mapping.
- No non-GET calls, resource lifecycle calls, or extra discovery requests occur.
- Invalid IDs cause no HTTP traffic.
- Minimal ID/name responses succeed with optional fields absent; explicit false/zero/empty optional values are preserved.
- Default environments are readable and their flag is returned accurately.
- Legacy `image` fallback and canonical `dockerImage` precedence work without defaults.
- Missing objects, forbidden/unauthorized errors, malformed JSON, wrong field types, missing identity/name, and mismatched IDs fail safely.
- Cancellation and deadline failures retain useful categories without leaking request URLs.
- Responses deliberately containing passwords, private keys, environment data, source/Compose contents, nested credentials, and unknown fields produce no corresponding output properties or leaked diagnostic text.
- Credential-bearing registry URLs are omitted.
- Framework-level invokes, not only direct typed handlers, validate decoding, optional values, output allowlisting, and schema registration.
- Existing resources and publishing metadata remain intact; generated SDK function exports and docs remain synchronized.

Use pinned tools and applicable Makefile checks: `make lint`, `make test_provider`, `make test_race`, `make provider`, `make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`, and `make docs_check`. Confirm expected generated changes are committed before drift checks that compare against git. Run `git diff --check` and inspect the final diff/status.

**Safety:** `.mise.toml` loads `.env` and sets `DOKPLOY_ACCEPTANCE=1`. Explicitly override that variable to `0` inside the mise-launched command environment for all test/build validation, including nested mise invocations. Do not read or print `.env`, credential variables, or protected IDs. Do not run live acceptance or publication workflows. Report unavailable checks instead of claiming they passed.

## Scope and completion

Out of scope: name lookups, lists/search, a generic database function, additional resource families, secret-output options, a Server managed resource, lifecycle changes, live acceptance coverage, release/publication, and unrelated refactoring.

Preserve the pre-existing untracked Schedule implementation plan and its existing design spec. Once implementation is finished, ask whether to retain or delete this task's spec, implementation plan, and related working documents; delete only explicitly approved task-specific files.

This is the design artifact, not implementation authorization. After the user reviews and approves this written spec, write the implementation plan and let the user choose its execution method.
