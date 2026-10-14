import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("Bitbucket guide documents exclusive secret modes and conditional identity", () => {
  const guide = read("../src/content/docs/guides/bitbucket-integrations.mdx");
  for (const phrase of ["apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "exactly one", "secret", "bitbucketId", "pulumi import"]) {
    assert.match(guide, new RegExp(phrase, "i"));
  }
  assert.match(guide, /config set --secret/i);
  assert.match(guide, /workspace/i);
  assert.match(guide, /empty string/i);
  assert.match(guide, /not guaranteed/i);
  assert.match(guide, /does not validate/i);
  assert.match(guide, /does not delete/i);
  assert.match(guide, /unmanaged workloads/i);
  assert.doesNotMatch(guide, /current Bitbucket.{0,50}compatible/i);
});

test("Bitbucket setup obtains credentials externally before Pulumi manages Dokploy configuration", () => {
  const guide = read("../src/content/docs/guides/bitbucket-integrations.mdx");
  const setupText = guide.match(/## Setup sequence([\s\S]*?)(?=\n## )/)?.[1];
  assert.ok(setupText, "guide must describe the setup sequence");
  const setup = setupText.replace(/\s+/g, " ");
  const credentials = setup.indexOf("Obtain the API token or app password in Bitbucket outside Pulumi");
  const dokployConfiguration = setup.indexOf("Then use Pulumi to create or manage the Dokploy Bitbucket configuration");
  assert.ok(credentials >= 0, "credentials are obtained from Bitbucket externally");
  assert.ok(dokployConfiguration >= 0, "Pulumi manages the Dokploy configuration afterward");
  assert.ok(credentials < dokployConfiguration, "external credentials precede Pulumi configuration");
  assert.match(setup, /no separate manual Dokploy integration or authorization step is required/i);
  assert.doesNotMatch(setup, /configure and authorize the integration outside Pulumi in Dokploy first/i);
  assert.match(guide, /current Bitbucket acceptance is not guaranteed/i);
});

test("Bitbucket guide and canonical source match the public API-token contract", () => {
  const schema = JSON.parse(read("../../provider/cmd/pulumi-resource-dokploy/schema.json"));
  const resource = schema.resources["dokploy:index:BitbucketIntegration"];
  assert.deepEqual(Object.keys(resource.inputProperties).sort(), ["apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName", "name"]);
  assert.deepEqual([...resource.requiredInputs].sort(), ["name"]);
  assert.equal(resource.inputProperties.apiToken.secret, true);
  assert.equal(resource.inputProperties.appPassword.secret, true);
  const guide = read("../src/content/docs/guides/bitbucket-integrations.mdx");
  for (const field of Object.keys(resource.inputProperties)) assert.ok(guide.includes(`\`${field}\``), `guide must document ${field}`);
  assert.match(guide, /\`name\` is\s+the only statically required input/);
  assert.match(guide, /Both credentials are secret values/);
  assert.doesNotMatch(guide, /\`(?:authId|gitProviderId|organizationId|accessToken|refreshToken|readiness)\`/);
  const yaml = read("../../examples/yaml/Pulumi.yaml");
  const example = yaml.match(/  managedBitbucketIntegration:\n([\s\S]*?)(?=\n  [A-Za-z][A-Za-z0-9]*:\n|\n[^ ]|$)/)?.[1];
  assert.ok(example, "canonical YAML must declare managedBitbucketIntegration");
  assert.match(example, /type: dokploy:index:BitbucketIntegration/);
  assert.match(example, /apiToken: \$\{bitbucketApiToken\}/);
  assert.match(example, /bitbucketEmail:/);
  assert.match(example, /bitbucketWorkspaceName:/);
  assert.doesNotMatch(example, /appPassword|bitbucketUsername/);
  assert.match(yaml, /bitbucketApiToken:[\s\S]*?secret: true/);
  assert.match(yaml, /managedBitbucketIntegrationId: \$\{managedBitbucketIntegration\.bitbucketId\}/);
  assert.match(guide, /config set --secret bitbucketApiToken/);
  assert.match(guide, /omitting.*workspace|omitted.*workspace/i);
  assert.match(guide, /empty.*workspace|workspace.*empty string/i);
  assert.match(guide, /does not validate connectivity/i);
  assert.match(guide, /does not delete workloads/i);
  assert.match(guide, /current Bitbucket acceptance is not guaranteed/i);
  assert.doesNotMatch(guide, /current Bitbucket.{0,50}compatible/i);
});
