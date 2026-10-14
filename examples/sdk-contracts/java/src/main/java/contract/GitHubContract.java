package contract;

import com.pulumi.resources.CustomResourceOptions;
import net.dimeski.pulumi.dokploy.GitHubIntegration;
import net.dimeski.pulumi.dokploy.GitHubIntegrationArgs;

final class GitHubContract {
    static void contract() {
        var args = GitHubIntegrationArgs.builder().name("existing").githubAppName("app").build();
        var resource = new GitHubIntegration("existing", args, CustomResourceOptions.builder().importId("relation-id-placeholder").build());
        var outputs = java.util.List.of(resource.name(), resource.githubAppName(), resource.githubId(), resource.gitProviderId(), resource.organizationId());
    }
}
