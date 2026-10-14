import * as dokploy from "@dimeskigj/pulumi-dokploy";
import * as pulumi from "@pulumi/pulumi";

export function githubContract() {
    const args: dokploy.GitHubIntegrationArgs = { name: "existing", githubAppName: "app" };
    const r = new dokploy.GitHubIntegration("existing", args, { import: "relation-id-placeholder" });
    const outputs: pulumi.Output<string>[] = [r.name, r.githubAppName, r.githubId, r.gitProviderId, r.organizationId];
    return outputs;
}
