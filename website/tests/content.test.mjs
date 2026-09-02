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
  "guides/backups.mdx",
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
   const resourceOrder = ["Project", "Environment", "Application", "Compose", "Postgres", "Redis", "Domain", "Destination", "Backup", "VolumeBackup", "SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "Configuration", "Complex Types"];
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
  const landing = await readFile(new URL("../src/content/docs/index.mdx", import.meta.url), "utf8");
  assert.match(landing, /link: getting-started\/installation\//);
  assert.match(landing, /link: reference\/project\//);
  assert.match(landing, /text: Get started/);
  assert.match(landing, /text: Resource reference/);
  assert.doesNotMatch(landing, /link: \/(getting-started|reference)\//);
});

test("landing page contains the required hierarchy and release-safe Registry wording", async () => {
  const landing = await readFile(new URL("../src/content/docs/index.mdx", import.meta.url), "utf8");
  assert.match(landing, /template: splash/);
  assert.match(landing, /pagefind: false/);
  assert.match(landing, /hero:/);
  assert.match(landing, /<CardGrid/);
  assert.match(landing, /Deploy Dokploy with Pulumi/);
  assert.match(landing, /Resource reference/);
  for (const language of ["TypeScript", "Python", "Go", "C#", "Java", "YAML"]) {
    assert.match(landing, new RegExp(`<Badge text="${language}"`), `landing must advertise ${language} support`);
  }
  const capabilities = landing.slice(landing.indexOf("## From intent to deployment"), landing.indexOf("## Write in the language"));
  assert.equal((capabilities.match(/<Card title="/g) ?? []).length, 8, "landing must define eight capability cards");
  assert.match(landing, /first release is published/i);
  assert.match(landing, /https:\/\/github\.com\/dimeskigj\/pulumi-dokploy/);
  assert.match(landing, /https:\/\/www\.pulumi\.com\/registry\/packages\/dokploy\//);
  const hierarchy = ["hero:", "<CardGrid", "## Write in the language", "## Provider guarantees", "github.com/dimeskigj", "www.pulumi.com/registry"].map((marker) => landing.indexOf(marker));
  assert.ok(hierarchy.every((position, index) => position >= 0 && (index === 0 || position > hierarchy[index - 1])), "landing sections must remain in canonical order");
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
  assert.match(databases, /MySQL/);
  assert.match(databases, /MariaDB/);
  assert.match(databases, /MongoDB/);
  assert.match(databases, /Redis/);
  assert.match(databases, /Project|Environment|environmentId/);
  assert.match(databases, /runtime|placement/i);
  assert.match(databases, /password/i);
  assert.match(databases, /import/i);
  assert.match(databases, /reference\/postgres/);
  assert.match(databases, /reference\/mysql/);
  assert.match(databases, /reference\/mariadb/);
  assert.match(databases, /reference\/mongodb/);
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

test("Backups guide describes destinations, database backups, and volume backups", async () => {
  const backups = await readFile(new URL("../src/content/docs/guides/backups.mdx", import.meta.url), "utf8");
  assert.match(backups, /Destination/);
  assert.match(backups, /defaults to `"s3"`/);
  assert.match(backups, /postgresId/);
  assert.match(backups, /mysqlId/);
  assert.match(backups, /mariadbId/);
  assert.match(backups, /mongoId/);
  assert.match(backups, /no equivalent for Redis/i);
  assert.match(backups, /applicationId/);
  assert.match(backups, /composeId/);
  assert.match(backups, /serviceName/);
  assert.match(backups, /replaces the Backup/);
  assert.match(backups, /replaces the VolumeBackup/);
  assert.match(backups, /write-only secret input/i);
  assert.match(backups, /reference\/destination/);
  assert.match(backups, /reference\/backup/);
  assert.match(backups, /reference\/volume-backup/);
});

test("curated internal links stay relative and sidebar routes are canonical", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  const curatedFiles = required.filter((path) => path !== "index.mdx");
  for (const path of required) {
    const source = await readFile(new URL(`../src/content/docs/${path}`, import.meta.url), "utf8");
    assert.doesNotMatch(source, /(?:href=["']|\]\()\/pulumi-dokploy\//);
    for (const match of source.matchAll(/\]\((\/[^)]*)\)/g)) {
      assert.fail(`${path} contains a root-relative internal link: ${match[1]}`);
    }
  }
  const canonicalRoutes = new Set([
    "/", "/getting-started/installation/", "/getting-started/first-deployment/",
    "/concepts/projects-and-environments/", "/concepts/sources/", "/concepts/lifecycle-and-state/", "/concepts/secrets/",
    "/guides/applications/", "/guides/compose/", "/guides/databases/", "/guides/domains/", "/guides/backups/", "/guides/imports/", "/guides/troubleshooting/",
    "/examples/", "/examples/complete/",
     "/reference/project/", "/reference/environment/", "/reference/application/", "/reference/compose/", "/reference/postgres/", "/reference/mysql/", "/reference/mariadb/", "/reference/mongodb/", "/reference/redis/", "/reference/domain/", "/reference/destination/", "/reference/backup/", "/reference/volume-backup/", "/reference/sshkey/", "/reference/registry/", "/reference/tag/", "/reference/project-tag/", "/reference/mount/", "/reference/configuration/", "/reference/types/",
    "/contributing/",
  ]);
  for (const match of config.matchAll(/link: "(\/[^\"]*)"/g)) {
    const route = match[1];
    assert.match(route, /^\/(?:[^/]+\/)*$/);
    assert.ok(canonicalRoutes.has(route), `sidebar route must be a canonical Starlight page: ${route}`);
  }
   assert.equal((config.match(/link: "\//g) ?? []).length, 37);
  assert.equal(curatedFiles.length, 14);
});

test("provider guides enforce exact schema discriminators and lifecycle statements", async () => {
  const applications = await readFile(new URL("../src/content/docs/guides/applications.mdx", import.meta.url), "utf8");
  assert.match(applications, /source:\s*\{ type: "docker"/);
  assert.match(applications, /source:\s*\{\n\s*type: "git"/);
  assert.match(applications, /source:\s*\{\n\s*type: "gitlab"/);
  const buildTypes = [...applications.matchAll(/build:\s*\{\s*type:\s*"([^"]+)"/g)].map((match) => match[1]);
  assert.ok(buildTypes.length >= 2);
  assert.ok(buildTypes.every((type) => ["nixpacks", "dockerfile"].includes(type)));
  assert.doesNotMatch(applications, /build:\s*\{ type: "docker" \}/);
  assert.match(applications, /A source discriminator change replaces the Application/);
  assert.match(applications, /environmentId.*serverId.*replacement-only/);

  const compose = await readFile(new URL("../src/content/docs/guides/compose.mdx", import.meta.url), "utf8");
  for (const variant of ["raw", "git", "gitlab"]) assert.equal((compose.match(new RegExp(`type: "${variant}"`, "g")) ?? []).length, 1);
  assert.match(compose, /fetch the selected raw or repository source, then redeploy and wait for deployment completion/);
  assert.match(compose, /deleteVolumesOnDestroy` defaults to `false`/);

  const domains = await readFile(new URL("../src/content/docs/guides/domains.mdx", import.meta.url), "utf8");
  assert.match(domains, /exactly one service target: an `applicationId` or a `composeId`/);
  assert.match(domains, /Changing the Application\/Compose target or `serviceName` replaces the Domain/);
  assert.match(domains, /Routing fields .*along with `enabled`, update in place/);
  assert.match(domains, /has no deployment-status polling/);
  assert.doesNotMatch(domains, /`environmentId`|`serverId`|deployment status/);
});

test("Compose raw quickstart nests composeFile under the raw source", async () => {
  const compose = await readFile(new URL("../src/content/docs/guides/compose.mdx", import.meta.url), "utf8");
  assert.match(compose, /source: \{ type: "raw", raw: \{ composeFile:/);
  assert.doesNotMatch(compose, /source: \{ type: "raw", composeFile:/);
});

test("database examples use secret-aware configuration access", async () => {
  const databases = await readFile(new URL("../src/content/docs/guides/databases.mdx", import.meta.url), "utf8");
  assert.match(databases, /config\.requireSecret\("databasePassword"\)/);
  assert.match(databases, /config\.requireSecret\("redisPassword"\)/);
  assert.doesNotMatch(databases, /pulumi\.secret\(config\.require\(/);
});

test("Examples sidebar uses the complete examples routes", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  const examples = config.slice(config.indexOf('label: "Examples"'), config.indexOf('label: "Contributing"'));
  assert.match(examples, /label: "Examples", link: "\/examples\/"/);
  assert.match(examples, /label: "Complete example", link: "\/examples\/complete\/"/);
  assert.doesNotMatch(examples, /getting-started\/first-deployment/);
});

test("new resource references and lifecycle guidance are published", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  for (const route of ["sshkey", "registry", "tag", "project-tag", "mount"]) {
    assert.match(config, new RegExp(`/reference/${route}/`));
  }
  const complete = await readFile(new URL("../src/content/docs/examples/complete.mdx", import.meta.url), "utf8");
  for (const marker of ["SSHKey", "sshKeyId", "Registry", "registryId", "Tag", "ProjectTag", "Mount", "bind", "volume", "file"]) {
    assert.match(complete, new RegExp(marker), `complete example documents ${marker}`);
  }
  const imports = await readFile(new URL("../src/content/docs/guides/imports.mdx", import.meta.url), "utf8");
  for (const resource of ["SSHKey", "Registry", "Tag", "ProjectTag", "Mount"]) assert.match(imports, new RegExp(resource));
  const applications = await readFile(new URL("../src/content/docs/guides/applications.mdx", import.meta.url), "utf8");
  assert.match(applications, /sshKeyId/);
  const lifecycle = await readFile(new URL("../src/content/docs/concepts/lifecycle-and-state.mdx", import.meta.url), "utf8");
  assert.match(lifecycle, /automatic mount redeployment/i);
  const databases = await readFile(new URL("../src/content/docs/guides/databases.mdx", import.meta.url), "utf8");
  assert.match(databases, /MongoDB/);
  assert.match(databases, /LibSQL/);
});
