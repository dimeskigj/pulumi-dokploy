import assert from "node:assert/strict";
import { setMocks } from "@pulumi/pulumi/runtime/mocks.js";
import { BitbucketIntegration } from "../../../sdk/nodejs/bin/bitbucketIntegration.js";
import { GiteaIntegration } from "../../../sdk/nodejs/bin/giteaIntegration.js";
import { GitLabIntegration } from "../../../sdk/nodejs/bin/gitLabIntegration.js";

let captured;
await setMocks({
    newResource(args) {
        if (args.type === "dokploy:index:GiteaIntegration"
            || args.type === "dokploy:index:BitbucketIntegration"
            || args.type === "dokploy:index:GitLabIntegration") {
            captured = args.inputs;
        }
        return { id: "offline-id", state: args.inputs };
    },
    call(args) {
        return args.inputs;
    },
});

const explicitEmpty = new GiteaIntegration("empty-internal-url", {
    name: "ordinary-name",
    clientId: "ordinary-client-id",
    clientSecret: "",
    giteaUrl: "https://git.example.test",
    redirectUri: "https://dokploy.example.test/callback",
    giteaInternalUrl: "",
});
await explicitEmpty.urn.promise();
function assertSecretValue(envelope, expectedValue, property) {
    assert.deepEqual(envelope, {
        "4dabf18193072939515e22adb298388d": "1b47061264138c4ac30d75fd1eb44270",
        value: expectedValue,
    }, `${property} must be forwarded as a Pulumi secret`);
}
assertSecretValue(captured.giteaInternalUrl, "", "explicit empty internal URL");
assertSecretValue(captured.clientSecret, "", "empty clientSecret");
assert.equal(captured.name, "ordinary-name", "ordinary name must remain nonsecret");
assert.equal(captured.clientId, "ordinary-client-id", "ordinary client ID must remain nonsecret");

const omitted = new GiteaIntegration("omitted-internal-url", {
    name: "ordinary-name",
    clientId: "ordinary-client-id",
    clientSecret: "credential",
    giteaUrl: "https://git.example.test",
    redirectUri: "https://dokploy.example.test/callback",
});
await omitted.urn.promise();
assert.equal(captured.giteaInternalUrl, undefined, "omitted internal URL stays omitted");

const bitbucket = new BitbucketIntegration("empty-bitbucket-credentials", {
    name: "ordinary-name",
    apiToken: "",
    appPassword: "",
});
await bitbucket.urn.promise();
assertSecretValue(captured.apiToken, "", "empty Bitbucket apiToken");
assertSecretValue(captured.appPassword, "", "empty Bitbucket appPassword");
assert.equal(captured.name, "ordinary-name", "Bitbucket name must remain nonsecret");

const emptyGitLab = new GitLabIntegration("empty-gitlab-secrets", {
    name: "ordinary-name",
    applicationId: "ordinary-application-id",
    applicationSecret: "",
    redirectUri: "https://dokploy.example.test/callback",
    gitlabInternalUrl: "",
});
await emptyGitLab.urn.promise();
assertSecretValue(captured.applicationSecret, "", "empty GitLab applicationSecret");
assertSecretValue(captured.gitlabInternalUrl, "", "explicit empty GitLab internal URL");
assert.equal(captured.name, "ordinary-name", "GitLab name must remain nonsecret");
assert.equal(captured.applicationId, "ordinary-application-id", "GitLab application ID must remain nonsecret");

const omittedGitLabInternalUrl = new GitLabIntegration("omitted-gitlab-internal-url", {
    name: "ordinary-name",
    applicationId: "ordinary-application-id",
    applicationSecret: "credential",
    redirectUri: "https://dokploy.example.test/callback",
});
await omittedGitLabInternalUrl.urn.promise();
assert.equal(captured.gitlabInternalUrl, undefined, "omitted GitLab internal URL stays omitted");

console.log("Node.js integration runtime contracts passed");
