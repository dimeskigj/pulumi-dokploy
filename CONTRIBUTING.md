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
| `NUGET_USER` | Set to the NuGet.org profile name (not email) that owns the Trusted Publishing policy for this workflow file. |
| `PYPI_API_TOKEN` | Create a PyPI API token scoped to the `pulumi-dokploy` project; use an account-scoped token for the first publication. |
| `JAVA_SIGNING_KEY` | Export the ASCII-armored private PGP key used to sign Maven artifacts, including the `BEGIN` and `END` lines. |
| `JAVA_SIGNING_PASSWORD` | Set the passphrase for `JAVA_SIGNING_KEY`. |
| `OSSRH_USERNAME` | Set the Sonatype Central publishing username or user-token name. |
| `OSSRH_PASSWORD` | Set the matching Sonatype Central password or user-token value. |

`CODECOV_TOKEN` is optional and enables authenticated coverage uploads.
`GITHUB_TOKEN` is created automatically for each workflow run; do not create a
repository secret for it.

NuGet and npm publishing use OIDC Trusted Publishing instead of a long-lived
API key or token: configure a Trusted Publishing policy on nuget.org and a
Trusted Publisher on npmjs.com for each release workflow file
(`prerelease.yml` and `release.yml`).

The manually dispatched live acceptance workflow uses separate protected
Dokploy and registry secrets in the `dokploy-acceptance` environment.
