package contract;

import com.pulumi.resources.CustomResourceOptions;
import net.dimeski.pulumi.dokploy.GitHubIntegration;
import net.dimeski.pulumi.dokploy.GitHubIntegrationArgs;
import net.dimeski.pulumi.dokploy.GiteaIntegration;
import net.dimeski.pulumi.dokploy.GiteaIntegrationArgs;
import net.dimeski.pulumi.dokploy.BitbucketIntegration;
import net.dimeski.pulumi.dokploy.BitbucketIntegrationArgs;

final class GitHubContract {
    static void contract() {
        var args = GitHubIntegrationArgs.builder().name("existing").githubAppName("app").build();
        var resource = new GitHubIntegration("existing", args, CustomResourceOptions.builder().importId("relation-id-placeholder").build());
        var outputs = java.util.List.of(resource.name(), resource.githubAppName(), resource.githubId(), resource.gitProviderId(), resource.organizationId());
    }

    static void giteaContract() {
        var args = GiteaIntegrationArgs.builder().name("gitea").giteaUrl("https://gitea.example.invalid")
                .clientId("client-id").clientSecret("secret-placeholder").redirectUri("https://dokploy.example.invalid/callback")
                .giteaInternalUrl("https://internal.example.invalid").organizationName("engineering").build();
        var resource = new GiteaIntegration("gitea", args, CustomResourceOptions.builder().importId("relation-id-placeholder").build());
        var outputs = java.util.List.of(resource.name(), resource.giteaUrl(), resource.clientId(), resource.clientSecret(), resource.redirectUri(),
                resource.giteaInternalUrl(), resource.organizationName(), resource.giteaId(), resource.gitProviderId(), resource.organizationId());
    }

    static void bitbucketContracts() {
        var apiArgs = BitbucketIntegrationArgs.builder().name("bitbucket-api").apiToken("token-placeholder")
                .bitbucketEmail("user@example.invalid").bitbucketWorkspaceName("workspace").build();
        var appPasswordArgs = BitbucketIntegrationArgs.builder().name("bitbucket-app-password").appPassword("password-placeholder")
                .bitbucketUsername("legacy-user").build();
        var api = new BitbucketIntegration("bitbucket-api", apiArgs, CustomResourceOptions.builder().importId("relation-id-placeholder").build());
        var appPassword = new BitbucketIntegration("bitbucket-app-password", appPasswordArgs, CustomResourceOptions.builder().importId("relation-id-placeholder").build());
        var outputs = java.util.List.of(api.name(), api.apiToken(), api.appPassword(), api.bitbucketEmail(), api.bitbucketUsername(),
                api.bitbucketWorkspaceName(), api.bitbucketId(), api.gitProviderId(), api.organizationId(), appPassword.name(), appPassword.bitbucketId());
    }
}
