# Contributing

1. Install the versions in `.mise.toml` with `mise install`.
2. Make changes in provider source and tests; use TDD for behavior changes.
3. Run `make lint`, `make check_openapi`, `make codegen`, `make build_sdks`, and `make test`.
4. Update the owned GitHub Actions workflows directly when CI behavior changes.
5. Ensure `git diff --check` is clean and include generated schema/SDK changes in the commit.

Pull requests should explain user-visible behavior and include regression tests. Acceptance tests
require protected repository secrets and are run manually by maintainers.

## Release credentials

Before creating a stable or prerelease tag, configure package publication
credentials under **Settings > Secrets and variables > Actions > Repository
secrets**:

| Secret | Setup |
| --- | --- |
| `NUGET_PUBLISH_KEY` | Create an API key at NuGet.org with push access to the `Pulumi.Dokploy` package. |
| `NPM_TOKEN` | Create an npm automation or granular access token with publish access to the `@dimeskigj/pulumi-dokploy` package. |
| `PYPI_API_TOKEN` | Create a PyPI API token scoped to the `pulumi-dokploy` project; use an account-scoped token for the first publication. |
| `JAVA_SIGNING_KEY` | Export the ASCII-armored private PGP key used to sign Maven artifacts, including the `BEGIN` and `END` lines. |
| `JAVA_SIGNING_PASSWORD` | Set the passphrase for `JAVA_SIGNING_KEY`. |
| `OSSRH_USERNAME` | Set the Sonatype Central publishing username or user-token name. |
| `OSSRH_PASSWORD` | Set the matching Sonatype Central password or user-token value. |

`CODECOV_TOKEN` is optional and enables authenticated coverage uploads.
`GITHUB_TOKEN` is created automatically for each workflow run; do not create a
repository secret for it.

The manually dispatched live acceptance workflow uses separate protected
Dokploy and registry secrets in the `dokploy-acceptance` environment.
