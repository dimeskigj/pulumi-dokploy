# Shared publication Task 4 report — Gitea + Bitbucket

Primary report for the serialized shared-publication task on
`feat/git-provider-integrations-rollout`, starting at `ba964e8b54ba90e93d534cc7c98bf7fbdeda5068`.

## Scope and result

- Kept the already-reviewed Gitea lifecycle and Task 4a provider registration intact; did not reimplement either.
- Added Bitbucket registration with `configuredClient`, additive inventories, and exact schema/provider-boundary tests. No additional Bitbucket annotations or dependency-wiring changes were needed: the registered preview and secret/identity assertions passed against the existing implementation.
- Canonically generated schema and all five SDKs for both resources; extended the maintained Go, Node.js, Python, .NET, and Java SDK-contract sources for both integrations. Constructors are referenced only by compile-contract functions; they are not run.
- Resolved the carried Bitbucket Diff coverage gap with per-input direct-diff cases, independent omission cases for email/username/workspace, and mode-switch assertions. Replaced wall-clock-only Create discovery cancellation with deterministic post-discovery cancellation, requiring one create and one completed post-create discovery and asserting no later calls.
- Lowercased the two Bitbucket lifecycle safe-prose strings flagged by ST1005. Error classification, inference partial-state behavior, and safe sanitized diagnostics remain unchanged.
- Updated the stale provider/lookup/registry resource inventories, without changing any existing resource or invoke assertions.

## Actual inventories

Before this publication commit, the integrated provider source exposed 25 resources and 12 invokes, while the committed generated schema snapshot still had 24 resources and 12 invokes. The selected OpenAPI operation inventory was already 153 paths. After this commit the generated and source inventories agree at 26 resources, 12 invokes, and the same 153 paths.

Final resources (26): `Application`, `Backup`, `BitbucketIntegration`, `Compose`, `Destination`, `Domain`, `Environment`, `GitHubIntegration`, `GitLabIntegration`, `GiteaIntegration`, `MariaDB`, `MongoDB`, `Mount`, `MySQL`, `Notification`, `Port`, `Postgres`, `Project`, `ProjectTag`, `Redis`, `Registry`, `SSHKey`, `Schedule`, `Server`, `Tag`, `VolumeBackup`.

Final invokes (12): `getApplication`, `getCompose`, `getEnvironment`, `getMariaDB`, `getMongoDB`, `getMySQL`, `getPostgres`, `getProject`, `getRedis`, `getRegistry`, `getSSHKey`, `getServer`.

`openapi/operations.txt` and generated OpenAPI/client sources were not changed. The exact inventory readout was produced from `provider/cmd/pulumi-resource-dokploy/schema.json` and `openapi/operations.txt` after canonical codegen.

## TDD and regression evidence

- Registration/schema/provider-boundary RED: `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration)' -count=1` failed because the Bitbucket schema token was absent and the registered integration server returned `Resource 'index:BitbucketIntegration' not found`. After additive registration, the focused Bitbucket suite passed.
- Focused final GREEN: `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration|BitbucketDiffCoversEveryInputAndIndependentOptionalOmissions|BitbucketUpdatePartialState)' -count=1` passed after canonical generation (`/tmp/opencode/shared-pub-focused-postcodegen.log`). This includes exact schema, registered dry-run identity/secrets, Create/Update infer-boundary cancellation and deadline classifications, and the new Diff/partial-state cases.
- The first full `make test_provider` exposed two stale additive-inventory assertions in `TestLookupSchemaContract` and `TestRegistryMetadata`; the tests were updated to expect 26 and name Bitbucket. `go test ./provider -run '^(TestLookupSchemaContract|TestRegistryMetadata|TestProviderRegistersProjectAndEnvironmentResources|TestSchemaHasExactlyTheMVPResources)$' -count=1` then passed (`/tmp/opencode/shared-pub-inventory-green.log`), and the subsequent full provider suite passed.
- Initial lint found an ineffectual test-helper client assignment in the deterministic discovery test; it was removed. The final lint passed with `0 issues`.
- A first lifecycle prose assertion used `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run '^TestBitbucketUpdatePartialState$' -count=1` and failed because it expected unsanitized intermediate text while `ResourceInitFailedError` correctly exposed the sanitized generic reason (`/tmp/opencode/shared-pub-red3.log`). The regression assertion was corrected to assert the sanitized reason and init-failed classification; safe diagnostic behavior was preserved. The source strings were lowercased and final lint passed.
- The direct Diff behavior was already correct; the carried review gap was test coverage only. The new direct cases were executed GREEN without changing the implementation.

## Commands, results, and logs

Every Go test, Make, SDK build, codegen, and compiler command below used the approved SAFE launcher with `TMPDIR=/tmp/opencode`. No live acceptance, credentials, `.env`, publication, push, or merge was used.

| Order | Command | Result | Full log |
| --- | --- | --- | --- |
| 1 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration)' -count=1` | Expected RED: Bitbucket schema/resource absent | `/tmp/opencode/shared-pub-red1.log` |
| 2 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make lint` | First run found the test helper's ineffectual assignment; after removing it, lint passed | `/tmp/opencode/shared-pub-lint-precodegen.log`, `/tmp/opencode/shared-pub-lint-green.log` |
| 3 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make codegen` | PASS; canonical source/schema/five SDKs generated | `/tmp/opencode/shared-pub-codegen.log` |
| 4 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make build_sdks` | PASS for Go, Python, Node.js, .NET, Java | `/tmp/opencode/shared-pub-build-sdks.log` |
| 5 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make test_sdk_contracts` | PASS for all five maintained language contracts; freshly published the Java SDK to local Maven | `/tmp/opencode/shared-pub-sdk-contracts.log` |
| 6 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make codegen` | PASS; restored canonical output after build normalization | `/tmp/opencode/shared-pub-codegen-final.log` |
| 7 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration|BitbucketDiffCoversEveryInputAndIndependentOptionalOmissions|BitbucketUpdatePartialState)' -count=1` | PASS | `/tmp/opencode/shared-pub-focused-final.log` |
| 8 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run go test ./provider -run '^(TestLookupSchemaContract|TestRegistryMetadata|TestProviderRegistersProjectAndEnvironmentResources|TestSchemaHasExactlyTheMVPResources)$' -count=1` | PASS after first full-suite run showed and fixed stale 25-resource assertions | `/tmp/opencode/shared-pub-inventory-green.log` |
| 9 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make test_provider` | First run exposed two stale inventories; final run PASS, provider and internal/client; live/release-gated cases skipped | `/tmp/opencode/shared-pub-test-provider.log`, `/tmp/opencode/shared-pub-test-provider-final.log` |
| 10 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make test_race` | PASS, provider and internal/client | `/tmp/opencode/shared-pub-test-race.log` |
| 11 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make lint` | PASS, 0 issues | `/tmp/opencode/shared-pub-lint-committed-candidate.log` |
| 12 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make provider` | PASS | `/tmp/opencode/shared-pub-provider-build.log` |
| 13 | `git diff --check`; explicit-path `git add`; `git diff --cached --check`; `git commit -m "feat: add guarded Bitbucket integration resource"` | PASS; commit `fe4832230d91f2a8451f68d02bb9991c98781fbe` | terminal output |
| 14 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make check_codegen` | PASS; normalization tests passed and committed SDK tree had no drift | `/tmp/opencode/shared-pub-check-codegen.log` |
| 15 | `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make check_openapi` | PASS; generated OpenAPI/client diff check clean | `/tmp/opencode/shared-pub-check-openapi.log` |
| 16 | `git show --check --format=fuller --no-patch HEAD`; `git diff HEAD^ HEAD --check`; `git status --short --untracked-files=all` | Commit checks PASS; only unrelated `?? task-5a-report.md` remains | terminal output |

SDK secret metadata inspection confirmed both Gitea `clientSecret`/`giteaInternalUrl` and Bitbucket `apiToken`/`appPassword` are annotated as secrets in generated Go, Node.js, Python, .NET, and Java sources (secret input wrappers and/or additional secret outputs as appropriate).

Build notes: generated .NET builds emitted two existing nullable warnings in `sdk/dotnet/Dokploy/Provider.cs` and a missing-readme package warning; Python normalized the development version during packaging. These did not fail builds/contracts. Canonical codegen after builds restored intended generated output.

## Commit, status, and remaining checks

- Commit: `fe4832230d91f2a8451f68d02bb9991c98781fbe` — `feat: add guarded Bitbucket integration resource`; 41 explicitly owned source, test, schema, SDK, and SDK-contract files.
- Post-commit `make check_codegen` and `make check_openapi` passed. Existing GitHub/GitLab resources, invokes, source variants, SDK fixtures, and helpers were preserved.
- `make test_examples` and `make docs_check` were not run: maintained SDK contracts were run as required, while broader examples/docs translation is outside this shared publication scope. Live acceptance was intentionally not run.
- One unrelated untracked root file, `task-5a-report.md`, appeared during work and remains untouched/unstaged. Thus the committed diff is clean, but `git status --short` still reports that unowned file; it was not deleted or included.
- Formatting used direct `gofmt -w` invocations during iteration before a final SAFE `gofmt -w` pass. Go test/build/Make/compiler validations used SAFE throughout; no formatter command read protected configuration or performed external operations.
- No task-specific plans/specs were created and no cleanup was performed. Approved specs and prior 4a reports remain untouched.

## Shared4b fix round 1 — Java input secrecy and Node.js empty values

Fix round base: `192df10b45a0216c68d7b80d634ee6d99a69d583`. Fix implementation commit: `2f5826e` (`fix: preserve integration secret and empty inputs`). This is a path-scoped follow-up to the prior reviewed shared-publication work; it does not change the approved guides, provider Go resources, schema, or OpenAPI source.

### Changes and runtime contracts

- Added schema-driven canonical generation normalization for only the declared secret string input properties of `GiteaIntegration` and `BitbucketIntegration`. Java Args builders now apply the pinned Java SDK's `Codegen.stringProp(...).secret().arg(...).getNullable()` handling to Gitea `clientSecret`/`giteaInternalUrl` and Bitbucket `apiToken`/`appPassword`. Using `getNullable()` preserves absent optional values.
- The same post-generation step replaces Node.js truthiness checks for those schema-secret inputs with `!== undefined`. Explicit `""` is therefore passed through `pulumi.secret(...)`; omitted optional URL stays `undefined`. The normalizer checks schema-declared names and string types, verifies expected generated target shapes, fails closed on missing/unexpected generated targets, and is covered for idempotence.
- Added offline Java Args runtime checks for literal and ordinary nonsecret `Output` credentials, secret propagation for all four fields, absent Gitea internal URL preservation, and nonsecret ordinary names/client IDs/URLs/redirect URI/Bitbucket identity and workspace settings.
- Added offline Node.js resource-mock checks for explicit empty secret internal URL, omitted URL, empty Gitea/Bitbucket credentials forwarded in Pulumi secret envelopes, and ordinary name/client ID values remaining plaintext. The existing compile-only fixtures remain compile-only; these checks are separate runtime entrypoints.

### TDD and verification

- Java RED against the generated pre-fix Args: the offline runtime runner failed with `AssertionError: Gitea clientSecret literal Output was not secret`.
- Node.js RED against the generated pre-fix SDK: the offline resource mock asserted the explicit empty internal URL and observed `undefined` instead of `""`.
- An initial Node mock run of the expanded Bitbucket case exposed a test-harness capture bug (the mock only saved Gitea inputs); the harness was corrected to capture both resource tokens. The final runtime run passes for both resources.
- `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run python3 -m unittest discover -s scripts -p test_normalize_generated.py` — PASS, 5 tests (`/tmp/opencode/shared4b-normalizer-final.log`). Tests cover schema-driven Java/Node transformations, unaffected ordinary fields, fail-closed target validation, and idempotence.
- `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make codegen` — PASS; schema and SDKs regenerated canonically (`/tmp/opencode/shared4b-codegen-final.log`). Re-running `normalize-generated.py integration-inputs` against the generated outputs produced identical SHA-256 manifests for both Java Args and both Node resource sources (`/tmp/opencode/shared4b-canonical-before.sha`, `/tmp/opencode/shared4b-canonical-after.sha`; `diff -u` clean).
- `env TMPDIR=/tmp/opencode /tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run sh -c 'make test_sdk_contracts > /tmp/opencode/shared4b-contracts-final2.log 2>&1; result=$?; grep -E "^\\[(go|nodejs|python|dotnet|java)\\]|runtime contracts passed|Build succeeded|BUILD SUCCESS|Error" /tmp/opencode/shared4b-contracts-final2.log | tail -n 30; exit $result'` — PASS for all five maintained SDK contracts. Node.js runtime contract printed `Node.js integration runtime contracts passed`; Java runtime Args contract printed `Java integration args runtime contracts passed`. This also built the affected Node SDK and published/compiled the affected Java SDK against the pinned APIs.
- `git diff --check` — PASS before commit. Commit `2f5826e` contains the ten task-owned source, regression-test, canonical generated SDK, and Make/test-runner files only.

Build notes: Java Gradle/Javadoc emitted existing missing-`@return`/`@param` warnings. .NET contract build emitted two pre-existing nullable warnings in generated `Provider.cs` and the existing package-readme warning; it succeeded with zero errors. No Go provider files changed, so prior provider/race/lint evidence remains applicable. No live tests, credentials, `.env`, push, merge, release, or cleanup were used. `make check_codegen` and `make check_openapi` are run after this fix/report commit and recorded below.

### Post-commit drift checks

- Pending at report authoring; append exact results after running the committed-output checks.
