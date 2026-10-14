# Bitbucket shared-publication Task 4 report

The complete joint Gitea4 + Bitbucket4b report is at:

`/home/dimeskigj/Projects/personal/pulumi-dokploy/.worktrees/git-provider-integrations-rollout/.superpowers/sdd/2026-10-13-gitea-integration/task-4-report.md`

Bitbucket outcome: registered the resource additively; verified exact six-input schema, only-name requiredness, secret metadata, registered-provider preview identities, and Create/Update infer partial-state cancellation/deadline classification; added direct Diff/dependency and deterministic discovery-cancellation coverage; lowercased the two safe lifecycle messages; generated all five SDKs and extended all five maintained SDK contracts. No Bitbucket source annotations/dependency changes were required beyond provider registration.

Joint final inventory is 26 resources, 12 invokes, and 153 selected API paths. Joint commit: `fe4832230d91f2a8451f68d02bb9991c98781fbe` (`feat: add guarded Bitbucket integration resource`).

Verification passed: focused Bitbucket provider tests, `make lint`, `make test_provider`, `make test_race`, `make provider`, `make build_sdks`, `make test_sdk_contracts` (fresh local Java SDK), `make check_codegen`, and `make check_openapi`. Exact commands/results and log paths are in the primary report. `make test_examples`, `make docs_check`, and live acceptance were not run; an unrelated untracked `task-5a-report.md` was left untouched.

## Shared4b fix round 1 pointer

Java input-secret propagation and Node.js explicit-empty preservation were addressed in fix commit `2f5826e` (`fix: preserve integration secret and empty inputs`), based on `192df10b45a0216c68d7b80d634ee6d99a69d583`. This follow-up changes canonical generation normalization and runtime contracts only; it does not modify Bitbucket provider code. The full fix details, exact command outcomes, and post-commit drift-check results are appended to the primary Gitea report at `.superpowers/sdd/2026-10-13-gitea-integration/task-4-report.md`.

## Shared4b fix round 2 pointer

The runner portability regression was fixed in `2280c71` (`fix: honor caller temp dirs in sdk tests`), based on `f43890c0e87e1d9bcc071523d2e5e00496b34e31`. Both SDK contract temporary directories now honor caller/platform defaults; the offline runner tests exercise actual creation/cleanup under a custom `TMPDIR`. Full script discovery and all five SDK contracts/runtime regressions passed. Exact commands and results are appended to the primary Gitea report at `.superpowers/sdd/2026-10-13-gitea-integration/task-4-report.md`.
