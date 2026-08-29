import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const required = [
  "index.mdx",
  "getting-started/installation.mdx",
  "getting-started/first-deployment.mdx",
  "concepts/projects-and-environments.mdx",
  "concepts/sources.mdx",
  "concepts/lifecycle-and-state.mdx",
  "concepts/secrets.mdx",
  "guides/applications.mdx",
  "guides/compose.mdx",
  "guides/databases.mdx",
  "guides/domains.mdx",
  "guides/imports.mdx",
  "guides/troubleshooting.mdx",
  "contributing.mdx",
];

test("all curated pages exist with title and description", async () => {
  for (const path of required) {
    const source = await readFile(new URL(`../src/content/docs/${path}`, import.meta.url), "utf8");
    assert.match(source, /^---\n[\s\S]*title:/);
    assert.match(source, /description:/);
  }
});

test("secret and destructive lifecycle guidance is explicit", async () => {
  const secrets = await readFile(new URL("../src/content/docs/concepts/secrets.mdx", import.meta.url), "utf8");
  assert.match(secrets, /pulumi config set --secret/);
  assert.match(secrets, /never log/i);
  const compose = await readFile(new URL("../src/content/docs/guides/compose.mdx", import.meta.url), "utf8");
  assert.match(compose, /deleteVolumesOnDestroy/);
  assert.match(compose, /false/);
});

test("sidebar keeps the canonical resource order and base-safe links", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  const resourceOrder = ["Project", "Environment", "Application", "Compose", "Postgres", "Redis", "Domain", "Configuration", "Complex Types"];
  const resources = config.slice(config.indexOf('label: "Resources"'), config.indexOf('label: "Examples"'));
  let previous = -1;
  for (const label of resourceOrder) {
    const position = resources.indexOf(`label: "${label}"`);
    assert.ok(position > previous, `${label} must follow the previous resource`);
    previous = position;
  }
  assert.match(config, /label: "Get Started"/);
  assert.match(config, /label: "Core Concepts"/);
  assert.match(config, /label: "Guides"/);
  assert.match(config, /label: "Examples"/);
  const hero = await readFile(new URL("../src/components/HomeHero.astro", import.meta.url), "utf8");
  assert.match(hero, /import \{ base \} from "\.\.\/site-config\.mjs"/);
  assert.match(hero, /withBase\("getting-started"\)/);
  assert.match(hero, /withBase\("reference"\)/);
  assert.match(hero, /Get started/);
  assert.match(hero, /referenceLabel/);
});

test("landing page contains the required hierarchy and release-safe Registry wording", async () => {
  const landing = await readFile(new URL("../src/content/docs/index.mdx", import.meta.url), "utf8");
  assert.match(landing, /template: splash/);
  assert.match(landing, /pagefind: false/);
  assert.match(landing, /<HomeHero/);
  assert.match(landing, /<CapabilityMap/);
  assert.match(landing, /Deploy Dokploy with Pulumi/);
  assert.match(landing, /Resource reference/);
  assert.match(landing, /TypeScript · Python · Go · C# · Java · YAML/);
  assert.equal((landing.match(/title: "/g) ?? []).length, 7, "landing must define seven capability cards");
  assert.match(landing, /first release is published/i);
  assert.match(landing, /https:\/\/github\.com\/gjorgjidimeski\/pulumi-dokploy/);
  assert.match(landing, /https:\/\/www\.pulumi\.com\/registry\/packages\/dokploy\//);
});

test("resource guides cover provider source variants and dependencies", async () => {
  const applications = await readFile(new URL("../src/content/docs/guides/applications.mdx", import.meta.url), "utf8");
  for (const variant of ["docker", "git", "gitlab"]) assert.match(applications, new RegExp(`type: [\\"']${variant}`));
  assert.match(applications, /public Git|public repository/i);
  assert.match(applications, /existing GitLab integration/i);
  assert.match(applications, /Project|Environment|environmentId/);
  assert.match(applications, /update/i);
  assert.match(applications, /replacement/i);
  assert.match(applications, /reference\/application/);

  const compose = await readFile(new URL("../src/content/docs/guides/compose.mdx", import.meta.url), "utf8");
  for (const variant of ["raw", "git", "gitlab"]) assert.match(compose, new RegExp(`type: [\\"']${variant}`));
  assert.match(compose, /fetch|redeploy/i);
  assert.match(compose, /Project|Environment|environmentId/);
  assert.match(compose, /reference\/compose/);
  assert.match(compose, /deleteVolumesOnDestroy/);

  const databases = await readFile(new URL("../src/content/docs/guides/databases.mdx", import.meta.url), "utf8");
  assert.match(databases, /Postgres/);
  assert.match(databases, /Redis/);
  assert.match(databases, /Project|Environment|environmentId/);
  assert.match(databases, /runtime|placement/i);
  assert.match(databases, /password/i);
  assert.match(databases, /import/i);
  assert.match(databases, /reference\/postgres/);
  assert.match(databases, /reference\/redis/);
});

test("Domain guide describes only supported targets and lifecycle behavior", async () => {
  const domains = await readFile(new URL("../src/content/docs/guides/domains.mdx", import.meta.url), "utf8");
  assert.match(domains, /exactly one|one target/i);
  assert.match(domains, /applicationId/);
  assert.match(domains, /composeId/);
  assert.match(domains, /serviceName/);
  assert.match(domains, /replace/i);
  assert.match(domains, /routing|enabled.*in.place|in.place.*enabled/i);
  assert.doesNotMatch(domains, /environmentId|serverId|deployment polling|waits for.*deployment/i);
  assert.match(domains, /reference\/domain/);
});
