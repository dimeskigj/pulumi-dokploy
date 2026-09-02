# Task 8 report

## Scope

Updated canonical examples, metadata/reference tests, curated documentation,
sidebar/reference model expectations, and regenerated the provider schema, all
five SDKs, all five language examples, and generated website reference/example
pages for SSHKey, Registry, Tag, ProjectTag, Mount, and Application registry/
SSH-key inputs.

## Commands and outcomes

All commands below were run in `/Users/gjorgjidimeski/Projects/pulumi-dokploy/.worktrees/ssh-registry-tags-mounts` unless noted.

* `git status --short && git log --oneline -10` — PASS; worktree was clean at
  start and the expected prior Task 7 commits were present.
* `mise exec -- go test ./provider -run 'TestRegistryMetadata|TestSchema' -count=1` —
  expected RED failure before implementation (`README missing "SSHKey"`).
* `mise exec -- go test ./examples -count=1` — expected setup failure because
  the example tests require the `all` build tag.
* `npm test --prefix website` — expected RED failures for the stale resource
  set, sidebar, and generated documentation.
* `mise exec -- make codegen` — PASS; schema and Go/Node.js/Python/.NET/Java
  SDKs regenerated. Pulumi emitted its existing secret Output warning and
  ephemeral-agent claim notice (see caveats).
* `mise exec -- make gen_examples` — PASS; all five language examples generated
  and normalized.
* `mise exec -- make docs_generate` — PASS; reference pages and complete
  example regenerated.
* `mise exec -- go test ./provider -run 'TestRegistryMetadata|TestSchema' -count=1` — PASS.
* `mise exec -- go test ./examples -tags=all -count=1` — PASS.
* `npm test --prefix website` — PASS; 43 tests passed.
* `mise exec -- go test ./openapi/cmd/normalize -count=1` — PASS.
* `mise exec -- make check_openapi` — PASS.
* `mise exec -- go test -short -v -count=1 ./provider/... ./internal/...` — PASS.
* `mise exec -- make test_race` — PASS.
* `mise exec -- make check_codegen` — expected pre-commit failure because
  generated SDK/schema changes were not committed yet; rerun after commit.
* `mise exec -- make build_sdks` — PASS. Existing generated .NET nullability
  warnings were emitted; no errors.
* `mise exec -- make test_examples` — PASS. Existing Java Javadoc/Maven and
  Gradle deprecation warnings were emitted; no errors.
* `mise exec -- make docs_check` — expected pre-commit failure at
  `check:generated`, because generated reference/example changes were not yet
  committed; the nested checks themselves were subsequently run successfully.
* `npm ci --prefix website && npm --prefix website run check` — PASS; Astro
  reported 0 errors, 0 warnings, 0 hints and the 43 website tests passed.
* `npm --prefix website run build` — PASS; 38 pages built.
* `npm --prefix website run test:built` — PASS; 2 built-site tests passed.
* `mise exec -- make docs_build` — PASS; 38 pages built.
* `python3 -m unittest scripts/test_normalize_ci.py -v` — PASS; 34 tests passed.
* `mise exec -- make lint` — FAIL due pre-existing Task 5/6 files outside this
  task: `provider/mount.go:54` goconst and `provider/mount_test.go:63` gofmt.
  These files were not modified because the brief forbids unrelated changes.
* `git diff --check` — expected pre-commit failure on whitespace emitted by
  regenerated Java SDK files; this is generator-owned output and was not hand
  edited. Rerun after commit.
* `git add ... && git commit -m "docs: publish SSH registry tag and mount resources"` —
  PASS; committed as `b88a683`.
* `mise exec -- make check_codegen` after commit — PASS; regeneration was
  deterministic and the schema/SDK tree matched HEAD. Pulumi repeated the
  existing warning and ephemeral-agent notice.
* `git diff --check` after commit — PASS.
* `npm --prefix website run check:generated` after commit — PASS.
* `git status --short` after verification — PASS; clean.

## Caveats

* The first `make codegen` invocation was accidentally issued from the main
  repository root. It produced no root changes; the correct generator commands
  were then run in this isolated worktree.
* Pulumi printed ephemeral-agent account notice with claim URL
  `https://app.pulumi.com/claim/01a06127-135b-79a4-a365-0ed3aa163a39` and a
  two-day expiry. No account claim was performed.
* Pulumi reported three high-severity `npm audit` findings during `npm ci` and
  Node reported an existing `undici` engine mismatch; neither blocked builds.
* Astro emitted its existing missing `src/icons` warning.
* The environment-gated live command was not run: no live Dokploy credentials
  were available. The exact command specified by the plan was therefore
  unavailable:

  `DOKPLOY_ENDPOINT="$DOKPLOY_ENDPOINT" DOKPLOY_API_KEY="$DOKPLOY_API_KEY" DOKPLOY_REGISTRY_URL="$DOKPLOY_REGISTRY_URL" DOKPLOY_REGISTRY_USERNAME="$DOKPLOY_REGISTRY_USERNAME" DOKPLOY_REGISTRY_PASSWORD="$DOKPLOY_REGISTRY_PASSWORD" DOKPLOY_REGISTRY_IMAGE_PREFIX="$DOKPLOY_REGISTRY_IMAGE_PREFIX" mise exec -- go test ./provider -run TestLive -v -count=1`

* The requested Luna reviewer/implementer dispatch was unavailable to this
  implementation subagent and was not performed.
