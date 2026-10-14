import com.pulumi.core.Output;
import com.pulumi.core.internal.OutputInternal;
import net.dimeski.pulumi.dokploy.BitbucketIntegrationArgs;
import net.dimeski.pulumi.dokploy.GiteaIntegrationArgs;
import net.dimeski.pulumi.dokploy.GitLabIntegrationArgs;

public final class RuntimeContract {
    private RuntimeContract() {}

    public static void main(String[] args) {
        var gitea = GiteaIntegrationArgs.builder()
                .name("ordinary-name")
                .clientId(Output.of("ordinary-client-id"))
                .clientSecret(Output.of(""))
                .giteaUrl(Output.of("https://git.example.test"))
                .redirectUri(Output.of("https://dokploy.example.test/callback"))
                .giteaInternalUrl(Output.of(""))
                .build();
        requireSecret(gitea.clientSecret(), "Gitea clientSecret literal Output");
        requireSecret(gitea.giteaInternalUrl().orElseThrow(), "Gitea empty internal URL Output");
        requirePlaintext(gitea.clientId(), "Gitea clientId");
        requirePlaintext(gitea.name(), "Gitea name");
        requirePlaintext(gitea.giteaUrl(), "Gitea URL");
        requirePlaintext(gitea.redirectUri(), "Gitea redirect URI");

        var literalGitea = GiteaIntegrationArgs.builder()
                .name("ordinary-name")
                .clientId("ordinary-client-id")
                .clientSecret("")
                .giteaUrl("https://git.example.test")
                .redirectUri("https://dokploy.example.test/callback")
                .giteaInternalUrl("")
                .build();
        requireSecret(literalGitea.clientSecret(), "Gitea literal clientSecret");
        requireSecret(literalGitea.giteaInternalUrl().orElseThrow(), "Gitea literal empty internal URL");

        var omittedInternalUrl = GiteaIntegrationArgs.builder()
                .name("ordinary-name")
                .clientId("ordinary-client-id")
                .clientSecret("credential")
                .giteaUrl("https://git.example.test")
                .redirectUri("https://dokploy.example.test/callback")
                .build();
        if (omittedInternalUrl.giteaInternalUrl().isPresent()) {
            throw new AssertionError("omitted Gitea internal URL must remain absent");
        }

        var gitlab = GitLabIntegrationArgs.builder()
                .name(Output.of("ordinary-name"))
                .applicationId(Output.of("ordinary-application-id"))
                .applicationSecret(Output.of("ordinary-application-secret"))
                .redirectUri(Output.of("https://dokploy.example.test/callback"))
                .gitlabInternalUrl(Output.of(""))
                .build();
        requireSecret(gitlab.applicationSecret(), "GitLab applicationSecret Output");
        requireSecret(gitlab.gitlabInternalUrl().orElseThrow(), "GitLab empty internal URL Output");
        requirePlaintext(gitlab.name(), "GitLab name");
        requirePlaintext(gitlab.applicationId(), "GitLab application ID");
        requirePlaintext(gitlab.redirectUri(), "GitLab redirect URI");

        var literalGitLab = GitLabIntegrationArgs.builder()
                .name("ordinary-name")
                .applicationId("ordinary-application-id")
                .applicationSecret("")
                .redirectUri("https://dokploy.example.test/callback")
                .gitlabInternalUrl("")
                .build();
        requireSecret(literalGitLab.applicationSecret(), "GitLab literal empty applicationSecret");
        requireSecret(literalGitLab.gitlabInternalUrl().orElseThrow(), "GitLab literal empty internal URL");

        var omittedGitLabInternalUrl = GitLabIntegrationArgs.builder()
                .name("ordinary-name")
                .applicationId("ordinary-application-id")
                .applicationSecret("credential")
                .redirectUri("https://dokploy.example.test/callback")
                .build();
        if (omittedGitLabInternalUrl.gitlabInternalUrl().isPresent()) {
            throw new AssertionError("omitted GitLab internal URL must remain absent");
        }

        var bitbucket = BitbucketIntegrationArgs.builder()
                .name(Output.of("ordinary-name"))
                .apiToken(Output.of(""))
                .appPassword(Output.of("ordinary-app-password"))
                .bitbucketEmail(Output.of("ordinary@example.test"))
                .bitbucketUsername(Output.of("ordinary-user"))
                .bitbucketWorkspaceName(Output.of("ordinary-workspace"))
                .build();
        requireSecret(bitbucket.apiToken().orElseThrow(), "Bitbucket empty apiToken Output");
        requireSecret(bitbucket.appPassword().orElseThrow(), "Bitbucket appPassword Output");
        requirePlaintext(bitbucket.name(), "Bitbucket name");
        requirePlaintext(bitbucket.bitbucketEmail().orElseThrow(), "Bitbucket email");
        requirePlaintext(bitbucket.bitbucketUsername().orElseThrow(), "Bitbucket username");
        requirePlaintext(bitbucket.bitbucketWorkspaceName().orElseThrow(), "Bitbucket workspace");

        var literalBitbucket = BitbucketIntegrationArgs.builder()
                .name("ordinary-name")
                .apiToken("")
                .appPassword("ordinary-app-password")
                .build();
        requireSecret(literalBitbucket.apiToken().orElseThrow(), "Bitbucket literal apiToken");
        requireSecret(literalBitbucket.appPassword().orElseThrow(), "Bitbucket literal appPassword");

        System.out.println("Java integration args runtime contracts passed");
    }

    private static void requireSecret(Output<String> value, String description) {
        if (!((OutputInternal<String>) value).isSecret().join()) {
            throw new AssertionError(description + " was not secret");
        }
    }

    private static void requirePlaintext(Output<String> value, String description) {
        if (((OutputInternal<String>) value).isSecret().join()) {
            throw new AssertionError(description + " was unexpectedly secret");
        }
    }
}
