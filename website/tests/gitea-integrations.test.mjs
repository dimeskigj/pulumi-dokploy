import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("Gitea guide documents manual authorization, secret configuration, and relation import", () => {
  const guide = read("../src/content/docs/guides/gitea-integrations.mdx");
  for (const phrase of ["clientSecret", "giteaInternalUrl", "giteaId", "pulumi import", "authorize", "does not create", "does not validate", "does not delete", "unmanaged workloads", "organizationName", "null"]) {
    assert.match(guide, new RegExp(phrase, "i"));
  }
  assert.match(guide, /config set --secret/i);
  assert.match(guide, /https:\/\/[^\s"`]+/);
  assert.doesNotMatch(guide, /Gitea(?:Integration)? source type/i);
});

test("Gitea setup orders external OAuth prerequisites, Pulumi configuration, and manual Dokploy authorization", () => {
  const guide = read("../src/content/docs/guides/gitea-integrations.mdx");
  const setupText = guide.match(/## Setup sequence([\s\S]*?)(?=\n## )/)?.[1];
  assert.ok(setupText, "guide must describe the full setup sequence");
  const setup = setupText.replace(/\s+/g, " ");
  const externalApp = setup.indexOf("Create the OAuth application in Gitea");
  const pulumiConfiguration = setup.indexOf("Pulumi creates and manages the Dokploy configuration");
  const manualAuthorization = setup.indexOf("After the Dokploy configuration exists, complete authorization manually in Dokploy");
  const workloadOperation = setup.indexOf("Only after authorization, use the integration in a separately supported workload operation");
  assert.ok(externalApp >= 0, "the Gitea OAuth app is an external prerequisite");
  assert.ok(externalApp < pulumiConfiguration, "the OAuth app must exist before Pulumi configuration");
  assert.ok(pulumiConfiguration < manualAuthorization, "Dokploy authorization follows managed configuration");
  assert.ok(manualAuthorization < workloadOperation, "authorization precedes a separate workload operation");
  assert.match(setup, /normal create does not require a pre-created Dokploy integration record/i);
  assert.doesNotMatch(guide, /complete authorization in Dokploy[^.]*before managing the integration/i);
});

test("Gitea guide and canonical source match the exact public and secret contract", () => {
  const schema = JSON.parse(read("../../provider/cmd/pulumi-resource-dokploy/schema.json"));
  const resource = schema.resources["dokploy:index:GiteaIntegration"];
  assert.deepEqual(Object.keys(resource.inputProperties).sort(), ["clientId", "clientSecret", "giteaInternalUrl", "giteaUrl", "name", "organizationName", "redirectUri"]);
  assert.deepEqual([...resource.requiredInputs].sort(), ["clientId", "clientSecret", "giteaUrl", "name", "redirectUri"]);
  assert.equal(resource.inputProperties.clientSecret.secret, true);
  assert.equal(resource.inputProperties.giteaInternalUrl.secret, true);
  const guide = read("../src/content/docs/guides/gitea-integrations.mdx");
  for (const field of Object.keys(resource.inputProperties)) assert.ok(guide.includes(`\`${field}\``), `guide must document ${field}`);
  assert.match(guide, /The first five are\s+required/);
  assert.match(guide, /client secret and internal URL are secret\s+values/i);
  assert.doesNotMatch(guide, /\`(?:organizationId|accessToken|refreshToken|scopes)\`/);
  const yaml = read("../../examples/yaml/Pulumi.yaml");
  const example = yaml.match(/  managedGiteaIntegration:\n([\s\S]*?)(?=\n  [A-Za-z][A-Za-z0-9]*:\n|\n[^ ]|$)/)?.[1];
  assert.ok(example, "canonical YAML must declare managedGiteaIntegration");
  assert.match(example, /type: dokploy:index:GiteaIntegration/);
  assert.match(example, /clientSecret:\n\s+fn::secret: \$\{giteaClientSecret\}/);
  assert.match(example, /giteaInternalUrl:\n\s+fn::secret: \$\{giteaInternalUrl\}/);
  assert.match(yaml, /managedGiteaIntegrationId: \$\{managedGiteaIntegration\.giteaId\}/);
  assert.doesNotMatch(example, /authorization|readiness|source:/i);
  assert.match(guide, /pulumi config set --secret giteaClientSecret/);
  assert.match(guide, /pulumi config set --secret giteaInternalUrl/);
  assert.match(guide, /does not perform this authorization step/i);
  assert.match(guide, /does not validate connectivity/i);
  assert.match(guide, /does not delete the external Gitea OAuth application[\s\S]*or delete workloads/i);
});
