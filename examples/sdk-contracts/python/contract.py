from pulumi_dokploy.git_hub_integration import GitHubIntegration, GitHubIntegrationArgs

args = GitHubIntegrationArgs(name="existing", github_app_name="app")
assert args.name is not None and args.github_app_name is not None
for prop in ("name", "github_app_name", "github_id", "git_provider_id", "organization_id"):
    assert hasattr(GitHubIntegration, prop)
