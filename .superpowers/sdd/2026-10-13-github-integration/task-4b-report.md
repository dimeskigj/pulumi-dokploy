# GitHub integration Task 4b report

## Task 4b1 status

Docs-only continuation completed and committed. Task 4b as a whole is not
complete: Task 4b2 still owns compiled contract fixtures for all five SDKs,
canonical language-example regeneration, and the full provider/SDK/OpenAPI/
codegen validation suite. No claim of overall Task 4 completion.

## Scope, files, and provenance

Committed README import/resource descriptions, canonical YAML/readme opt-in
import guidance, website guide/navigation/inventory and route tests, and
source-generated GitHubIntegration reference plus complete example page. The
canonical YAML remains without an active GitHubIntegration resource. The
reference and complete example were generated using `SAFE make docs_generate`
(existing website generators); no generated output was hand-edited.

Owned files in the Task 4b1 commit:
`README.md`, `examples/yaml/Pulumi.yaml`, `examples/yaml/README.md`,
`website/astro.config.mjs`, `website/scripts/generate-examples.mjs`,
`website/scripts/reference-model.mjs`,
`website/src/content/docs/examples/complete.mdx`,
`website/src/content/docs/guides/github-integrations.mdx`,
`website/src/content/docs/reference/github-integration.mdx`,
`website/tests/content.test.mjs`,
`website/tests/reference-model.test.mjs`,
`website/tests/github-integrations.test.mjs`.

## Commands/results

- `SAFE node --test website/tests/*.test.mjs` — passed, 70 tests.
- `SAFE go test ./examples -tags=yaml -run 'TestCanonicalYAML' -count=1` — passed.
- `SAFE make docs_generate` — passed; generated outputs from repository source.
- `git diff --check` — passed before commit.
- The first source-test run found two expected inventory misses after the
  additive GitHub resource/sidebar routes (resource count 23→24, and missing
  canonical GitHub routes). Fixed only those assertions; the subsequent full
  source suite passed.
- Previously `SAFE make docs_check` failed because route/resource assertions
  had not yet been updated and because generated changes were uncommitted.
  After the intentional docs commit, it was rerun as required; see outcome below.

## Commit(s)

- `<pending>` — Task 4b1 docs/source/generated outputs.
- `<pending>` — concrete website inventory assertion fixes, if separated.

## Post-commit docs verification

`SAFE make docs_check` was run after the docs commit. Its exact final outcome,
log path, and any focused follow-up commit are to be recorded here before
reporting completion.

## Self-review / remaining concerns

Reviewed that setup/authorization occurs manually in Dokploy, Pulumi import
uses the Dokploy relation ID (`githubId`), Create is unsupported and not
included in canonical YAML, `name`/`githubAppName` are subsequent managed
configuration, incomplete app-name settings need repair, and destroy only
removes the Dokploy relation while warning about unmanaged workloads. README
and YAML use reserved placeholders; no real IDs or credentials were added.

Task 4b2 remains: maintained compiled SDK contract fixtures in Node.js, Python,
Go, .NET, and Java; generated language examples; and the assigned full offline
provider/build/OpenAPI/codegen/example validation.
