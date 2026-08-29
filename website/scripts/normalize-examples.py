from pathlib import Path

REPLACEMENTS = {
    "examples/nodejs/index.ts": {
        'config.requireObject<any>("dokploy:endpoint")': 'config.require("dokploy:endpoint")',
        'config.getSecretObject<any>("dokploy:apiKey")': 'config.requireSecret("dokploy:apiKey")',
        'config.requireObject<any>("dokploy:apiKey")': 'config.requireSecret("dokploy:apiKey")',
        "\nconst gitlabIntegration =": "\nconst gitlabIntegrationConfig =",
        "\nconst gitlabProject =": "\nconst gitlabProjectConfig =",
        "\nconst gitlabOwner =": "\nconst gitlabOwnerConfig =",
        "\nconst gitlabNamespace =": "\nconst gitlabNamespaceConfig =",
        "\nconst gitlabRepository =": "\nconst gitlabRepositoryConfig =",
        "\nconst gitBranch =": "\nconst gitBranchConfig =",
        "export const gitlabIntegration0 = gitlabIntegration": "export const gitlabIntegration = gitlabIntegrationConfig",
        "export const gitlabProject0 = gitlabProject": "export const gitlabProject = gitlabProjectConfig",
        "export const gitlabOwner0 = gitlabOwner": "export const gitlabOwner = gitlabOwnerConfig",
        "export const gitlabNamespace0 = gitlabNamespace": "export const gitlabNamespace = gitlabNamespaceConfig",
        "export const gitlabRepository0 = gitlabRepository": "export const gitlabRepository = gitlabRepositoryConfig",
        "export const gitBranch0 = gitBranch": "export const gitBranch = gitBranchConfig",
    },
    "examples/python/__main__.py": {
        'config.require_object("dokploy:endpoint")': 'config.require("dokploy:endpoint")',
        'config.require_secret_object("dokploy:apiKey")': 'config.require_secret("dokploy:apiKey")',
        'config.require_object("dokploy:apiKey")': 'config.require_secret("dokploy:apiKey")',
    },
    "examples/go/main.go": {
        'var dokployEndpoint interface{}\n\t\tcfg.RequireObject("dokploy:endpoint", &dokployEndpoint)': '_ = cfg.Require("dokploy:endpoint")',
        'var dokployApiKey interface{}\n\t\tcfg.RequireSecretObject("dokploy:apiKey", &dokployApiKey)': '_ = cfg.RequireSecret("dokploy:apiKey")',
        'var dokployApiKey interface{}\n\t\tcfg.RequireObject("dokploy:apiKey", &dokployApiKey)': '_ = cfg.RequireSecret("dokploy:apiKey")',
    },
    "examples/dotnet/Program.cs": {
        'config.RequireObject<dynamic>("dokploy:endpoint")': 'config.Require("dokploy:endpoint")',
        'config.RequireSecretObject<dynamic>("dokploy:apiKey")': 'config.RequireSecret("dokploy:apiKey")',
        'config.RequireObject<dynamic>("dokploy:apiKey")': 'config.RequireSecret("dokploy:apiKey")',
    },
    "examples/java/src/main/java/generated_program/App.java": {
        'config.requireSecretObject("dokploy:apiKey", com.pulumi.core.TypeShape.map(String.class, Object.class))': 'config.requireSecret("dokploy:apiKey")',
        'config.requireObject("dokploy:apiKey", com.pulumi.core.TypeShape.map(String.class, Object.class))': 'config.requireSecret("dokploy:apiKey")',
        'ctx.export("gitlabIntegration0",': 'ctx.export("gitlabIntegration",',
        'ctx.export("gitlabProject0",': 'ctx.export("gitlabProject",',
        'ctx.export("gitlabOwner0",': 'ctx.export("gitlabOwner",',
        'ctx.export("gitlabNamespace0",': 'ctx.export("gitlabNamespace",',
        'ctx.export("gitlabRepository0",': 'ctx.export("gitlabRepository",',
        'ctx.export("gitBranch0",': 'ctx.export("gitBranch",',
    },
}

for filename, replacements in REPLACEMENTS.items():
    path = Path(filename)
    content = path.read_text()
    for old, new in replacements.items():
        content = content.replace(old, new)
    path.write_text(content)
