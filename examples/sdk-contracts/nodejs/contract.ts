import * as dokploy from "@dimeskigj/pulumi-dokploy";
import * as pulumi from "@pulumi/pulumi";

export function githubContract() {
    const args: dokploy.GitHubIntegrationArgs = { name: "existing", githubAppName: "app" };
    const r = new dokploy.GitHubIntegration("existing", args, { import: "relation-id-placeholder" });
    const outputs: pulumi.Output<string>[] = [r.name, r.githubAppName, r.githubId, r.gitProviderId, r.organizationId];
    return outputs;
}

export function giteaContract() {
    const args: dokploy.GiteaIntegrationArgs = {
        name: "gitea", giteaUrl: "https://gitea.example.invalid", clientId: "client-id",
        clientSecret: "secret-placeholder", redirectUri: "https://dokploy.example.invalid/callback",
        giteaInternalUrl: "https://internal.example.invalid", organizationName: "engineering",
    };
    const r = new dokploy.GiteaIntegration("gitea", args, { import: "relation-id-placeholder" });
    const outputs: pulumi.Output<string>[] = [r.name, r.giteaUrl, r.clientId, r.clientSecret, r.redirectUri, r.giteaId, r.gitProviderId, r.organizationId];
    const optionalOutputs: pulumi.Output<string | undefined>[] = [r.giteaInternalUrl];
    const group: pulumi.Output<string>[] = [r.organizationName];
    return [...outputs, ...optionalOutputs, ...group];
}

export function bitbucketContracts() {
    const apiArgs: dokploy.BitbucketIntegrationArgs = {
        name: "bitbucket-api", apiToken: "token-placeholder", bitbucketEmail: "user@example.invalid", bitbucketWorkspaceName: "workspace",
    };
    const appPasswordArgs: dokploy.BitbucketIntegrationArgs = {
        name: "bitbucket-app-password", appPassword: "password-placeholder", bitbucketUsername: "legacy-user",
    };
    const api = new dokploy.BitbucketIntegration("bitbucket-api", apiArgs, { import: "relation-id-placeholder" });
    const appPassword = new dokploy.BitbucketIntegration("bitbucket-app-password", appPasswordArgs, { import: "relation-id-placeholder" });
    const outputs: pulumi.Output<string>[] = [api.name, api.bitbucketId, api.gitProviderId, api.organizationId, appPassword.name, appPassword.bitbucketId];
    const optionalOutputs: pulumi.Output<string | undefined>[] = [api.apiToken, api.appPassword, api.bitbucketEmail, api.bitbucketUsername, api.bitbucketWorkspaceName];
    return [...outputs, ...optionalOutputs];
}
