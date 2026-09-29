# Agent guidance

This repository builds the Pulumi Dokploy provider in Go and generates its schema and Go, Node.js, Python, .NET, and Java SDKs. Read `CONTRIBUTING.md` and `tests/README.md` for development and test safety details. Use the versions pinned in `.mise.toml` (`mise install`) and prefer the Makefile targets below over ad hoc commands.

## Before changing files

- Read the relevant provider, OpenAPI, SDK, example, or website code and its tests. Keep changes focused; add regression tests for behavior changes.
- Treat generated files as generated. Change their source and regenerate rather than hand-editing output. `make codegen` replaces the SDK directories; `make check_openapi` regenerates OpenAPI outputs. Inspect existing uncommitted changes before running either target, and do not overwrite someone else's work.
- Keep API keys, passwords, private URLs, resource IDs, and other secrets out of source, logs, examples, and reports. Do not commit `.env` or real stack credentials.
- Never run live acceptance tests against production or as an incidental local check. They mutate a real Dokploy server and require explicit opt-in, protected credentials, and the safety procedure in `tests/README.md`.

## Validate before finishing

Run checks relevant to the files changed; do not claim an unrun check passed. The PR pipeline in `.github/workflows/build.yml` and `.github/workflows/lint.yml` is the source of truth. In particular:

| Changed area | Local checks to run when applicable |
| --- | --- |
| Go provider or internal code | `make lint`, `make test_provider`, `make test_race`, `make provider` |
| OpenAPI source or generated client | `make check_openapi` |
| Provider schema or SDK generation | `make check_codegen`, `make build_sdks`, `make test_examples` |
| SDKs or examples | `make build_sdks`, `make test_examples` (or the affected language targets for a narrow change) |
| Website or generated reference docs | `make docs_check` |
| Dependency or license-sensitive changes | `make govulncheck`, `make license` |

CI also checks schema compatibility on pull requests, builds each SDK, tests language examples, and checks for unexpected generated-file changes. Include generated schema/SDK changes when source changes require them. Run `git diff --check` and inspect `git status --short` and the relevant diff before finishing; distinguish your changes from pre-existing work. If a check is unavailable, too costly for the scope, or requires CI-only tooling, report that explicitly rather than treating it as passed. Do not run release or publication workflows as routine validation. The live acceptance workflow and released-package `release-smoke` workflow are manually dispatched, not routine PR checks.

## SDD document cleanup

When implementation is finished, identify any task-specific SDD design specs, implementation plans, or related working documents created for that work. Ask the user whether to retain or delete them; do not delete them without explicit approval. Only remove the approved, related documents, and preserve unrelated documentation and any pre-existing user changes. If none were created, no cleanup is needed.
