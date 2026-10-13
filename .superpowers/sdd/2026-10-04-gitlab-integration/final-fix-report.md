# Final GitLab integration fix wave

Base: `b8cce32`. Commit: the focused commit containing this report (recorded in final response).

## Scope and evidence

- `provider/gitlab_integration.go`: reject query (including bare `?`) and fragment (including bare `#`) on public/internal base URLs in both Check and apply validation. Keep callback validation separate; accept encoded path delimiters and internal basic auth. Errors use fixed, value-free text.
- `provider/gitlab_integration_test.go`: Check and concrete Create/Update rejection cases for both fields and all four component forms; verify no client invocation, safe messages, valid encoded paths/basic auth/callback, plus three required-input empty cases.
- `provider/gitlab_integration_lifecycle_test.go`: successful update changes `name` and confirms the changed value from post-update readback.
- `internal/client/gitlab_contract_test.go`: assert refresh token presence/exact decoded value; cover empty create, malformed update, JSON remove acknowledgments.
- `examples/yaml_test.go`: reject both OAuth token properties in the canonical managed resource.

RED: `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test ./provider -run '^TestGitLabIntegrationBaseURLComponents$' -count=1` exited 1: all eight public/internal query, bare query, fragment, and bare fragment Check cases unexpectedly had `Failures: []` (line 141 before formatting).

GREEN: `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test ./provider ./internal/client -run 'TestGitLabIntegration|TestGitLabGeneratedClientContract' -count=1` passed both packages; `/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/safe-run go test -tags yaml ./examples -run '^TestCanonicalYAMLGitLabIntegration$' -count=1` passed. Initial combined attempt without `-tags yaml` reported build constraints exclude all Go files in examples; initial broader focused attempt caught an overly broad assertion against the `applicationSecret` property name, corrected before GREEN.

Broader verification (all via the same safe-run launcher):

- `sh -c 'GOLANGCI_LINT_CACHE=/tmp/opencode/gitlab-integration-tools-ses-ef758d7d/lint-cache-final make lint'` in fresh isolated cache: `0 issues`.
- `make test_provider`: passed (`provider`, `internal/client`; generated and provider/cmd have no tests).
- `make test_race`: passed (`provider`, `internal/client`; generated and provider/cmd have no tests).
- `make provider`: passed; built local provider binary (ignored output).
- `go test -tags yaml ./examples -count=1`: passed.
- `git diff --check`: passed.

Self-review: No schema field, annotation, default, OpenAPI, SDK or website description changed; schema/codegen/docs regeneration was deliberately not run (it replaces generated directories and is unnecessary for value validation). Inspected full scoped diff and status; only five feature source/test paths and this report are in scope. URL parser component checks distinguish raw delimiters from percent-encoded path content; errors never interpolate raw URL or secret. No live acceptance or release tasks were run. Remaining limitation: no live server validation (intentionally prohibited); existing behavioral tests and isolated checks cover this fix. SDD documents retained per no-cleanup instruction; controller handles any future document decision.

## Final review fixes (base `ffa434d`)

### Finding 1: duplicate example generator / provenance

- Reproduced before fix: `/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run make -n gen_examples` exited 0 but printed GNU Make's `overriding recipe for target 'gen_examples'` and `ignoring old recipe`; it selected the inline recipe rather than the isolated script. Log: `/tmp/opencode/gitlab-gen-examples-red.log`.
- `Makefile` now has a single `gen_examples: codegen` target invoking `scripts/gen-examples.sh`. The existing isolated temporary `PULUMI_HOME`, local provider plugin installation, pinned Pulumi/mise selectors, and SAFE nested mise remain in that script. GitLab-era local SDK preparation remains there (Node/Python/Go and .NET/Java local SDK setup); converted Java/.NET compatibility rewrites were moved to `scripts/normalize-converted-examples.py`.
- Added `TestGenExamplesUsesIsolatedCanonicalGenerator`, checking unique target definition, script invocation, isolation/install setup, converter and local SDK normalization references.
- RED: SAFE `go test ./examples -tags=yaml -run '^TestGenExamplesUsesIsolatedCanonicalGenerator$' -count=1` failed because it found 2 recipes.
- Attempted canonical generation through SAFE launcher. The recursive target reached Pulumi's YAML converter, which tried to fetch the `yaml` converter plugin and failed with GitHub API HTTP 403 rate limit (`could not find latest version for provider yaml`). No generated translation outputs were changed, and website complete-example embed was not regenerated. Generation/provenance parity is explicitly blocked; do not claim generation passed. Log: `/tmp/opencode/gitlab-gen-examples-red.log` captures the initial duplicate override evidence; converter failure was returned by the later command (exit 1).
- Safety deviation: this mistaken `make -n` attempt did not set the required `TMPDIR` and the wrapper allowed the converter to attempt a GitHub API request. Its temp `PULUMI_HOME` was under the default `/tmp` and removed by the script's cleanup trap. No authenticated/private/live Dokploy request was made. No further generator invocation was made.

### Finding 2: README resource inventory

- Consolidated the duplicate inventory in `README.md` to one listing of all 23 current resources, including Port, Notification, Server, Schedule, and GitLabIntegration.
- Preserved adjacent GitLab authorization, managed-vs-external relation ID, and guide reference; retained surrounding backup, workload, SSH, and baseline resource guidance.

### Verification / limitations

- A post-fix SAFE focused test attempt initially used `make -n gen_examples`; GNU Make executed the recursive recipe, which encountered the converter plugin 403 and removed the generated language directories before reaching conversion. All five tracked generated directories were immediately restored from their known-clean pre-run state with `git restore -- examples/dotnet examples/go examples/java examples/nodejs examples/python`. No unrelated paths were restored or changed.
- Final focused regression: SAFE `go test ./examples -tags=yaml -run '^TestGenExamplesUsesIsolatedCanonicalGenerator$' -count=1` exited 0 (`ok ... 0.002s`). Source syntax checks `sh -n scripts/gen-examples.sh` and `python3 -m py_compile scripts/normalize-converted-examples.py` exited 0. `git diff --check` exited 0.
- SAFE `make test_provider` passed on retry (exit 0; log `/tmp/opencode/gitlab-final-test-provider.log`). The first invocation occurred while generation had removed the example directories and failed only the fixture tests for absent `examples/java/pom.xml` and `examples/go/go.mod`; it passed after restore.
- `make gen_examples`, `make test_examples`, `make docs_check`, `make test_provider`, `make check_codegen`, and `make check_openapi` have not been verified in this fix wave. Do not infer success from prior adoption-report evidence. Generated output parity and website embed refresh remain unverified and blocked on YAML converter plugin availability.
- No further generator, live service, credential, release, or protected operation attempted after the plugin error. No approved TMPDIR generator run completed, because converter plugin resolution failed before translating canonical YAML. Full final status/commit/check evidence follows after commit.

### Commit / final state

- Implementation commit: `cefc5a0 fix: consolidate GitLab example generation provenance`.
- Report-only evidence follow-up: recorded separately after implementation.
- Final `git diff --check` exited 0; status was clean after implementation commit. Regeneration and the remaining checks listed above are concerns/blockers, not claimed passes.
