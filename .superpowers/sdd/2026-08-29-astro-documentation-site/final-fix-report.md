# Astro documentation final-fix report

## Scope

Addressed all blocking and minor findings from the supplied review package:

- Corrected the TypeScript raw Compose source shape and added a semantic regression assertion.
- Marked `dokploy:apiKey` secret in canonical YAML using the valid namespaced `secret`/`value` form; normalized all generated language programs and refreshed the generated MDX example. Tests cover canonical YAML and generated secret accessors.
- Changed database and Redis passwords to `config.requireSecret` and added assertions.
- Corrected Examples sidebar routes and added exact route assertions.
- Tightened reference token validation to exact `dokploy:index:<identifier>` syntax with negative cases.
- Added built-site accessibility assertions for property tables/regions and language tabs/panels.
- Completed atomic restoration and cleanup-path assertions.
- Corrected secrets wording and imports terminology to “Pulumi resource type token.”

No plan, design spec, SDD ledger, or tracked raw logs were modified.

## Red/green evidence

The initial focused run failed on the new Compose nesting, database accessor, Examples sidebar, YAML secret shape, token validation, and cleanup assertion. After implementation and correction of the generated Pulumi YAML-compatible namespaced config form, the focused suite passed:

```text
mise exec -- node --test website/tests/reference-model.test.mjs website/tests/content.test.mjs website/tests/generate-examples.test.mjs website/tests/render-reference.test.mjs
34 passed, 0 failed
```

The final website test suite passed 42/42 tests, including the new semantic and atomic-path assertions.

## Generation evidence

```text
mise exec -- make gen_examples
completed successfully
```

The canonical YAML and all five generated language project YAML files now carry the secret API-key config. Generated TypeScript, Python, Go, C#, and Java programs use secret-aware API-key accessors. `website/src/content/docs/examples/complete.mdx` was regenerated from those tracked sources. `npm run check:generated` passed within `make docs_check`.

## Verification

Fresh post-commit verification used mise Node `v24.18.0`:

```text
mise exec -- node --version
v24.18.0

mise exec -- make test_examples
passed; Go, Python, TypeScript, .NET, and Java example checks succeeded

mise exec -- make docs_check
passed; generation clean, Astro check 0 errors/0 warnings/0 hints,
42 Node tests passed, production build generated 26 pages, built-site tests 2/2 passed

python3 -m unittest scripts/test_normalize_ci.py -v
Ran 33 tests in 0.033s — OK

mise exec -- make lint
0 issues.

git diff --check
passed
```

Final branch status after cleanup:

```text
## feature/astro-documentation-site
```

The final commit range is `22d31e6..HEAD` (this report is included in the final commit).

Required Pages setting after merge: “Set repository Settings > Pages > Build and deployment > Source to GitHub Actions.”

## Concerns

- `make test_examples` reports existing non-failing .NET nullable warnings and Maven/Gradle deprecation/Javadoc warnings; it completed successfully with zero errors.
- Pulumi generation emitted its environment’s ephemeral-agent claim notice during `make gen_examples`; no credentials or claim material was committed.
