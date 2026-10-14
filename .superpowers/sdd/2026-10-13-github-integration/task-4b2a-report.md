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
