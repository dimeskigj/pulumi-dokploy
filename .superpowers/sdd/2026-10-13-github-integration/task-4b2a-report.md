# Task 4b2a report

Added maintained, compile-only SDK contracts beneath `examples/sdk-contracts/{go,nodejs,python,dotnet,java}` and hooked `test_sdk_contracts` into `make test_examples`. The resource constructor contracts are helper methods and are never called; Python only constructs args and inspects generated properties. The relation-ID import placeholder is not a real resource ID.

## Focused checks

All commands ran through `/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run`.

| Language | Command(s) | Result |
|---|---|---|
| Go | `go test ./...` in `examples/sdk-contracts/go` | PASS, compiled fixture |
| Node.js | `npm install --package-lock=false --ignore-scripts --no-audit --no-fund`; `npx tsc --noEmit` | PASS |
| Python | venv pip install `-e sdk/python`; `python -m py_compile contract.py`; `python -c 'import contract'` | PASS |
| .NET | `dotnet build Contract.csproj --nologo` | PASS, 0 warnings/errors |
| Java | `mise exec java@temurin-17.0.20+8 maven@3.9.11 -- mvn -q package -DskipTests` | PASS |

`python3 -m unittest scripts.test_sdk_contract_runner`: PASS (3 tests; Make hook and all five fixture directories/outside-generation preservation). The focused runner performed all five compilation/import checks successfully. No `make test_examples`/full suite rerun: that suite belongs to the separate controller validation and had already been running per task instructions. No Pulumi program/deployment, resource constructor, live server, `.env`, publisher, or release operation was used.

## Review

Owned paths: `Makefile`, `.superpowers/sdd/2026-10-13-github-integration/task-4b2a-report.md`, `scripts/test_sdk_contract_runner.py`, `scripts/test_sdk_contracts.py`, and `examples/sdk-contracts/` (five language fixtures). Fixtures are outside generated `examples/{go,nodejs,python,dotnet,java}` locations; `scripts/gen-examples.sh` leaves them untouched. Example generated program sources were not changed. Reviewed generated SDK surface for field names and pinned compiler/dependency conventions. `git diff --check` passed. No concerns identified. The combined/full Make validation remains controller-owned.

## Fixround 1 (review of 2fc99ec)

RED/GREEN evidence: `SAFE python3 -m unittest scripts.test_sdk_contract_runner` initially failed three behavioral assertions: hardcoded launcher was captured in the child command, Python env path was not unique/caller-temp, and Make launched the fixture hook before local Java publication. Implemented environment-inherited direct subprocess invocation with Dokploy environment variables removed and `DOKPLOY_ACCEPTANCE=0`, `TemporaryDirectory`-scoped Python venv cleanup, Java SDK publication before the test_examples hook, and `install_java_sdk` as the standalone target prerequisite. Regression suite now uses mocked child executions and verifies caller temp location, unique venv paths/cleanup, five language compiler/import calls, fail-fast propagation, safety env, and Java ordering.

Commands were run as actual local executions through `/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run` (the runner itself no longer knows or requires that path):

- `SAFE python3 -m unittest scripts.test_sdk_contract_runner` — PASS, 6 tests. Full output: `/tmp/opencode/task4b2a-fixround1-unit.log`.
- `SAFE make test_sdk_contracts` — PASS. Ran existing Java 11 Gradle `publishToMavenLocal`, canonical Java example package, then all five fixtures. This verifies Java against freshly generated and locally published SDK, not a stale Maven artifact.
- `SAFE python3 scripts/test_sdk_contracts.py` — PASS, all fixture compilers/imports; detailed output: `/tmp/opencode/task4b2a-fixround1-fixtures.log`.
- `git diff --check` — PASS.

Self-review: runner has no fixed launcher or fixed temp location; `TemporaryDirectory` uses Python's TMPDIR-aware standard library and cleans its environment on success/failure; every child receives filtered Dokploy variables and forced-off acceptance. `make test_examples` now runs contracts after local Java publication. Java standalone execution depends on the local SDK installation step. No remote publication, environment-file access, deployments, or live calls. No generated files are part of the change.

## Fixround 2 (generator preservation regression)

Added `GeneratorPortabilityTests.test_sdk_contract_fixtures_survive_real_example_replacement`, which invokes the actual `scripts/gen-examples.sh` in a temporary fake project. It seeds nested unique-content fixtures for all five languages and stale outputs in every canonical language directory, stubs conversion/tool commands, verifies each fixture remains byte-identical and present after the generator runs, and verifies each stale canonical output was replaced with expected generated markers. It does not invoke production codegen, actual SDK generation, network publication, or deployment.

Verification via `/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run`:

- RED was not applicable: this is strictly added test coverage for the existing intended generator behavior; the new test passed on first run, confirming the generator preserves the outside-path fixtures.
- `SAFE python3 -m unittest scripts.test_gen_examples.GeneratorPortabilityTests.test_sdk_contract_fixtures_survive_real_example_replacement` — PASS, 1 test.
- `SAFE python3 -m unittest -v scripts.test_gen_examples scripts.test_sdk_contract_runner` — PASS, 12 tests (all generator portability and runner unit tests). Named output: `/tmp/opencode/task4b2a-fixround2-unit.log`.
- `git diff --check` — PASS.

Self-review: only this behavioral test and report changed in fixround 2; no generator production implementation or prior runner fixes changed. Existing fake tooling writes only under a temporary harness root and keeps generator's real removal/conversion loop under test. No `.env`, live calls, or additional generated suite was used.
