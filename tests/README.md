# Live acceptance tests

## Purpose

Live acceptance tests use a real Dokploy server. They create and delete test
resources, so they can change server state and consume server capacity.

**Warning:** Never run these tests against a production server. Use a dedicated
acceptance server with protected credentials and no unrelated resources.

## Test layers

The **Direct-provider lifecycle tests** exercise provider resources against the
Dokploy API. They run in four independent provider tiers:

- `TestLiveTier1ControlPlane` checks control-plane resources.
- `TestLiveTier2Workloads` checks applications, Compose services, domains,
  mounts, dispatches, source integrations, and disabled schedules.
- `TestLiveTier3Databases` checks database lifecycles.
- `TestLiveTier4Backups` checks backup and volume-backup lifecycles.

The **Pulumi Automation API smoke test**, `TestAccLifecycleSmoke`, exercises a
small Pulumi program through preview, update, refresh, and cleanup. It checks
provider discovery and Automation API behavior. It does not replace the
broader direct-provider coverage.

## Safety rules

- Run one provider tier at a time with `-parallel=1`.
- Permit only one heavy operation at a time. The harness enforces this rule.
- Keep backups disabled unless the command is the backup tier.
- Use reserved test hosts and names. Do not target an existing resource.
- Keep cleanup bounded and verify that owned resources are absent.
- Do not modify pre-existing resources.
- Do not print credentials, secret values, full environment dumps, private URLs,
  or resource IDs in terminal output or reports.

## Prerequisites

Install Go `1.26.6`, the pinned Pulumi CLI `3.259.0`, and the repository's
local provider binary.

**Process prerequisites** are the variables that enable a live test process:

- `DOKPLOY_ACCEPTANCE=1` opts into live execution.
- `DOKPLOY_ENDPOINT` identifies the dedicated Dokploy server.
- `DOKPLOY_API_KEY` authenticates the test run.

**Workflow/operator safety setup** must configure
`DOKPLOY_ACCEPTANCE_STOP_FILE` before the first tier. The variable names the
writable stop-marker path that the workflow and operator inspect between tiers.
It is safety setup, not a process prerequisite.

The server must support the resources selected by the tier. The operator must
have permission to create and delete only the reserved test resources.
Set `DOKPLOY_ACCEPTANCE_SERVER_ID` when Tier 1 or Tier 2 coverage requires an
explicit server scope.

## Local setup

1. Run `mise install` to install the pinned tools.
2. Run `make provider` to build the local provider binary.
3. Add the provider directory with `export PATH="$PWD/bin:$PATH"`.
4. Export `DOKPLOY_ENDPOINT` from a protected shell or credential store.
5. Export `DOKPLOY_API_KEY` from a protected shell or credential store.
6. Export `DOKPLOY_ACCEPTANCE=1` to opt in to live tests.
7. Choose a writable, absent path for `DOKPLOY_ACCEPTANCE_STOP_FILE`.
8. Export `DOKPLOY_ACCEPTANCE_STOP_FILE` with that path.
9. Remove the stop-marker path before the first tier.

The shell may source a protected credential file. Go test code must not read
`.env` or parse `.env`; the process receives credentials through its
environment.

## Test commands

Run the commands in this order. After every command, inspect
`DOKPLOY_ACCEPTANCE_STOP_FILE`. Stop before the next command if the marker
exists.

```bash
mise exec -- go test ./provider -run TestLiveTier1ControlPlane -parallel=1 -count=1 -v
mise exec -- go test ./provider -run TestLiveTier2Workloads -parallel=1 -count=1 -v
mise exec -- go test ./provider -run TestLiveTier3Databases -parallel=1 -count=1 -v
mise exec -- go test ./provider -run TestLiveTier4Backups -parallel=1 -count=1 -v
mise exec -- go test ./tests -run TestAccLifecycleSmoke -parallel=1 -count=1 -v
```

The final command is the Pulumi smoke. Run it only after all four provider
tiers finish and their stop-marker checks pass.

## Optional coverage

Tier 2 Schedule coverage uses only its disposable application and Compose targets,
always sends `enabled:false`, and never invokes manual command execution. Server
and Dokploy-server schedules skip unless
`DOKPLOY_ACCEPTANCE_ALLOW_SERVER_SCHEDULES=1` and
`DOKPLOY_ACCEPTANCE_SERVER_ID` are both set. This additional opt-in is an operator
assertion that the server scope is dedicated to acceptance, has no unrelated
resources, and (for `dokploy-server`) that the Dokploy host itself is dedicated.
The server ID scopes the `server` case; it is a safety prerequisite, not a target
ID sent to the `dokploy-server` case. Never enable this opt-in for production.
Schedules are deleted and absence verified before workload target cleanup,
including fallback cleanup for an acknowledged-create ID with a partial-state
error. If creation returns no acknowledged identity, the fixture stops the tier
and sets the stop marker for operator investigation; it never retries, searches
by name, or deletes an unverified identity. Offline tests do not establish the
deployed Schedule response contract or live compatibility.

Registry coverage requires `DOKPLOY_REGISTRY_URL`,
`DOKPLOY_REGISTRY_USERNAME`, `DOKPLOY_REGISTRY_PASSWORD`, and, when needed,
`DOKPLOY_REGISTRY_IMAGE_PREFIX`. Complete Registry mutation coverage additionally
requires four distinct, valid values: `DOKPLOY_REGISTRY_UPDATED_URL`,
`DOKPLOY_REGISTRY_UPDATED_USERNAME`, `DOKPLOY_REGISTRY_UPDATED_PASSWORD`, and
`DOKPLOY_REGISTRY_UPDATED_IMAGE_PREFIX`; otherwise that mutation subtest skips.
Destination provider mutation requires a separately validated provider value in
`DOKPLOY_ACCEPTANCE_DESTINATION_UPDATED_PROVIDER`; otherwise only the other
Destination fields are mutated. GitLab coverage requires
`DOKPLOY_GITLAB_INTEGRATION_ID`, `DOKPLOY_GITLAB_PROJECT_ID`,
`DOKPLOY_GITLAB_OWNER`, `DOKPLOY_GITLAB_NAMESPACE`,
`DOKPLOY_GITLAB_REPOSITORY`, and `DOKPLOY_GITLAB_BRANCH`, plus
`DOKPLOY_GITLAB_USERNAME` and `DOKPLOY_GITLAB_TOKEN`. Tier 2 receives all
GitLab variables from the protected workflow environment. The test skips each
integration when its prerequisite is absent.

MongoDB replica coverage requires `DOKPLOY_ACCEPTANCE_ALLOW_REPLICAS=1`; the
database tier skips replica sets without that opt-in. The
`DOKPLOY_CUSTOM_CERT_RESOLVER` variable enables the active custom certificate
resolver coverage in Tier 2. When absent, that case skips. Other optional
server-scope variables must remain protected and must not be copied into a
report. An absent optional prerequisite produces a skip, not a provider
failure.

## Cleanup and stop behavior

Each test registers fallback cleanup and checks explicit resource absence after
deletion. Cleanup contexts are independent, so a timed-out destroy does not
prevent stack removal or the next cleanup attempt. The harness treats an
already-absent resource as cleaned.

The stop marker is created only when cleanup fails or a confirmed server-health failure occurs. An ordinary heavy-operation failure does not create the marker.
Tier gates inspect the marker after each tier. Do not run a later heavy tier
when the marker exists. Preserve the marker and the sanitized failure
classification for the report.

## Result classification

- **Pass:** The selected lifecycle completed and cleanup verified absence.
- **Skip:** A declared opt-in or prerequisite was absent.
- **Provider defect:** The provider violated the documented API or lifecycle
  contract after safe reproduction.
- **Dokploy server defect:** The server returned behavior inconsistent with its
  deployed contract or rejected a valid provider request.
- **Test environment limitation:** Tools, access, credentials, or server
  capacity prevented execution without proving a provider or server defect.

An ordinary resource failure does not stop an independent tier. Cleanup failure
or a server-health stop condition does stop later heavy tiers through the stop
marker.

## Reports

Use [`docs/bugs/README.md`](../docs/bugs/README.md) for bug-report fields and
triage. Use the run record at
[`docs/bugs/2026-09-05-live-acceptance-run.md`](../docs/bugs/2026-09-05-live-acceptance-run.md)
as the report-shape example.

Record sanitized evidence, each tier's duration, every skip reason, and cleanup
status. Include the safe command, expected and actual result, tool versions,
and endpoint hostname only. Redact credentials, IDs, private URLs, request
payloads, and environment dumps.
