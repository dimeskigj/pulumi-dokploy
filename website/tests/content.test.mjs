import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const execFileAsync = promisify(execFile);

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
  "guides/schedules.mdx",
  "guides/ports.mdx",
  "guides/imports.mdx",
  "guides/lookups.mdx",
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
   const resourceOrder = ["Project", "Environment", "Application", "Compose", "Postgres", "Redis", "Domain", "Destination", "Backup", "VolumeBackup", "Schedule", "SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "Port", "Configuration", "Complex Types"];
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

const lookupRoutes = ["get-project", "get-environment", "get-application", "get-compose", "get-postgres", "get-mysql", "get-mariadb", "get-mongodb", "get-redis", "get-server", "get-registry", "get-ssh-key"];

test("lookup TypeScript guide snippet compiles with the CommonJS example and generated SDK", async () => {
  const guide = await readFile(new URL("../src/content/docs/guides/lookups.mdx", import.meta.url), "utf8");
  const snippet = guide.match(/```ts\n([\s\S]*?)\n```/)?.[1];
  assert.ok(snippet, "lookup guide must include a TypeScript code fence");
  const exampleDirectory = fileURLToPath(new URL("../../examples/nodejs/", import.meta.url));
  // Keep the temporary program inside the example package so NodeNext uses its
  // CommonJS package boundary and resolves its installed generated SDK.
  const temporary = await mkdtemp(join(exampleDirectory, ".lookup-guide-test-"));
  try {
    await writeFile(join(temporary, "index.ts"), `${snippet}\n`);
    // The example links the SDK source; route its Pulumi peer dependency to the
    // example's existing installation rather than requiring SDK-local npm install.
    await writeFile(join(temporary, "tsconfig.json"), JSON.stringify({
      extends: "../tsconfig.json",
      files: ["index.ts"],
      compilerOptions: {
        noEmit: true,
        baseUrl: "..",
        paths: { "@pulumi/pulumi": ["node_modules/@pulumi/pulumi"], "@pulumi/pulumi/*": ["node_modules/@pulumi/pulumi/*"] },
      },
    }));
    const compiler = fileURLToPath(new URL("../../examples/nodejs/node_modules/typescript/bin/tsc", import.meta.url));
    try {
      await execFileAsync(process.execPath, [compiler, "--project", join(temporary, "tsconfig.json")], { cwd: exampleDirectory });
    } catch (error) {
      assert.fail(`lookup guide TypeScript does not compile:\n${error.stdout || error.stderr || error.message}`);
    }
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
});

test("lookup guide and sidebar link every generated function without altering resources", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  const guide = await readFile(new URL("../src/content/docs/guides/lookups.mdx", import.meta.url), "utf8");
  assert.match(guide, /^---\ntitle:/);
  assert.match(guide, /description:/);
  for (const phrase of [/getProject/, /getEnvironment/, /LookupProject/, /ID.only|by ID/i, /import/i, /adopt/i, /refresh/i, /preview/i, /not.found/i, /authoriz/i, /non.secret/i, /managed resource/i]) assert.match(guide, phrase);
  assert.match(config, /label: "Lookups", link: "\/guides\/lookups\/"/);
  const functions = config.slice(config.indexOf('label: "Functions"'), config.indexOf('label: "Examples"'));
  let previous = -1;
  for (const route of lookupRoutes) {
    const position = functions.indexOf(`link: "/reference/${route}/"`);
    assert.ok(position > previous, `${route} must follow the previous function`);
    previous = position;
    const page = await readFile(new URL(`../src/content/docs/reference/${route}.mdx`, import.meta.url), "utf8");
    assert.match(page, /^---\ntitle:/);
    assert.match(page, /## Inputs[\s\S]*## Outputs/);
    assert.match(page, /read-only/i);
  }
});

test("landing page contains the required hierarchy and publication-safe Registry wording", async () => {
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
  assert.match(landing, /Registry listing.*pending/i);
  assert.doesNotMatch(landing, /first release is published/i);
  assert.match(landing, /https:\/\/github\.com\/dimeskigj\/pulumi-dokploy/);
  assert.doesNotMatch(landing, /https:\/\/www\.pulumi\.com\/registry\/packages\/dokploy\//);
  const hierarchy = ["hero:", "<CardGrid", "## Write in the language", "## Provider guarantees", "github.com/dimeskigj", "Registry listing"].map((marker) => landing.indexOf(marker));
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
  assert.match(backups, /`backup\.create` returns no new backup ID/i);
  assert.match(backups, /provider waits for one uniquely matching schedule to appear on the target/i);
  assert.match(backups, /ambiguity or visibility timeout can occur after remote success/i);
  assert.match(backups, /may successfully create the schedule before identity discovery times out/i);
  assert.match(backups, /inspect Dokploy before retrying because the schedule may already exist/i);
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
    "/guides/applications/", "/guides/compose/", "/guides/databases/", "/guides/domains/", "/guides/backups/", "/guides/schedules/", "/guides/ports/", "/guides/imports/", "/guides/lookups/", "/guides/troubleshooting/",
    "/examples/", "/examples/complete/",
     "/reference/project/", "/reference/environment/", "/reference/application/", "/reference/compose/", "/reference/postgres/", "/reference/mysql/", "/reference/mariadb/", "/reference/mongodb/", "/reference/redis/", "/reference/domain/", "/reference/destination/", "/reference/backup/", "/reference/volume-backup/", "/reference/sshkey/", "/reference/registry/", "/reference/tag/", "/reference/project-tag/", "/reference/mount/", "/reference/port/", "/reference/configuration/", "/reference/types/",
    "/contributing/",
    "/reference/schedule/",
    ...lookupRoutes.map((slug) => `/reference/${slug}/`),
  ]);
  for (const match of config.matchAll(/link: "(\/[^\"]*)"/g)) {
    const route = match[1];
    assert.match(route, /^\/(?:[^/]+\/)*$/);
    assert.ok(canonicalRoutes.has(route), `sidebar route must be a canonical Starlight page: ${route}`);
  }
    assert.equal((config.match(/link: "\//g) ?? []).length, 53);
    assert.equal(curatedFiles.length, 16);
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

test("complete example actively wires SSH, registry, and secret mount inputs", async () => {
  const yaml = await readFile(new URL("../../examples/yaml/Pulumi.yaml", import.meta.url), "utf8");
  assert.match(yaml, /genericGitApplication:[\s\S]*?type: dokploy:index:Application/);
  assert.match(yaml, /genericGitApplication:[\s\S]*?type: git[\s\S]*?sshKeyId: \$\{sshKey\.sshKeyId\}/);
  assert.match(yaml, /fileMountContent:[\s\S]*?secret: true/);
  assert.match(yaml, /content:\s*\n\s*fn::secret: \$\{fileMountContent\}/);
  assert.doesNotMatch(yaml, /fn::secret: APP_ENV=staging/);
});

test("Schedule reference and guide are published in the sidebar", async () => {
  const references = await readdir(new URL("../src/content/docs/reference/", import.meta.url));
  assert.ok(references.includes("schedule.mdx"), "generated Schedule reference must exist");
  const reference = await readFile(new URL("../src/content/docs/reference/schedule.mdx", import.meta.url), "utf8");
  assert.match(reference, /title: "Schedule"/);
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  assert.match(config, /label: "Schedule", link: "\/reference\/schedule\/"/);
  assert.match(config, /label: "Schedules", link: "\/guides\/schedules\/"/);
});

test("Schedule YAML sample stays disabled, application-scoped, and secret", async () => {
  const yaml = await readFile(new URL("../../examples/yaml/Pulumi.yaml", import.meta.url), "utf8");
  const sample = yaml.match(/^  applicationSchedule:\n[\s\S]*?(?=^  \w|$(?![\s\S]))/m)?.[0];
  assert.ok(sample, "disabled Schedule sample must exist");
  assert.match(sample, /type: dokploy:index:Schedule/);
  assert.match(sample, /scheduleType: application/);
  assert.match(sample, /applicationId: \$\{application\.applicationId\}/);
  assert.match(sample, /enabled: false/);
  assert.match(sample, /command:\s*\n\s*fn::secret:/);
  const complete = await readFile(new URL("../src/content/docs/examples/complete.mdx", import.meta.url), "utf8");
  assert.match(complete, /dokploy:index:Schedule/);
  assert.match(complete, /applicationSchedule/);
});

test("Schedule guide warns about command execution and the unverified live contract", async () => {
  const pages = await readdir(new URL("../src/content/docs/guides/", import.meta.url));
  assert.ok(pages.includes("schedules.mdx"), "Schedule safety guide must exist");
  const guide = await readFile(new URL("../src/content/docs/guides/schedules.mdx", import.meta.url), "utf8");
  assert.match(guide, /enabled schedules execute shell commands/i);
  assert.match(guide, /enabled.*defaults to.*false/i);
  assert.match(guide, /command.*script.*secret/i);
  assert.match(guide, /unverified.*live|live.*unverified/i);
  assert.match(guide, /dedicated non-production server/i);
  assert.match(guide, /reference\/schedule/);
});

test("Schedule guide distinguishes schema secrecy from Java builder outputs", async () => {
  const guide = await readFile(new URL("../src/content/docs/guides/schedules.mdx", import.meta.url), "utf8");
  assert.match(guide, /command.*script.*secret.*Pulumi schema/);
  assert.match(guide, /Java.*ordinary outputs/);
  assert.match(guide, /Output\.ofSecret/);
  assert.match(guide, /\.command\(Output\.ofSecret/);
  assert.match(guide, /\.script\(Output\.ofSecret/);
  assert.doesNotMatch(guide, /secret inputs in every generated SDK/);
});

test("Port reference and guide are published", async () => {
  const config = await readFile(new URL("../astro.config.mjs", import.meta.url), "utf8");
  assert.match(config, /label: "Port", link: "\/reference\/port\/"/);
  assert.match(config, /label: "Ports", link: "\/guides\/ports\/"/);
  const guide = await readFile(new URL("../src/content/docs/guides/ports.mdx", import.meta.url), "utf8");
  assert.match(guide, /reference\/port/);
  const reference = await readFile(new URL("../src/content/docs/reference/port.mdx", import.meta.url), "utf8");
  for (const field of ["applicationId", "publishedPort", "targetPort", "protocol", "publishMode", "portId"]) assert.match(reference, new RegExp(`"name":"${field}"`));
  assert.match(reference, /"defaultValue":"tcp"/);
  assert.match(reference, /"defaultValue":"ingress"/);
  const types = await readFile(new URL("../src/content/docs/reference/types.mdx", import.meta.url), "utf8");
  assert.match(types, /Port protocol: tcp or udp/);
  assert.match(types, /Port publish mode: ingress or host/);
});

test("Port guide explains configuration-only lifecycle and import limitations", async () => {
  const guide = await readFile(new URL("../src/content/docs/guides/ports.mdx", import.meta.url), "utf8");
  for (const marker of [/configuration.only/i, /do not deploy|does not deploy/i, /after deleting/i, /Compose/i, /firewall/i, /availability/i, /400/, /permission/i, /1\.\.65535/, /ingress/i, /host/i, /pulumi import dokploy:index:Port mapping <port-id>/, /application.*relation/i, /parent.*resource option/i, /404.*NOT_FOUND|NOT_FOUND.*404/i, /confirmed absence/i]) assert.match(guide, marker);
  assert.match(guide, /null|None|undefined/i);
  assert.match(guide, /omission|default/i);
  assert.match(guide, /explicit null/i);
});

test("Port canonical and generated examples are present", async () => {
  const yaml = await readFile(new URL("../../examples/yaml/Pulumi.yaml", import.meta.url), "utf8");
  assert.match(yaml, /applicationPort:[\s\S]*?type: dokploy:index:Port[\s\S]*?publishedPort: 8081[\s\S]*?targetPort: 80/);
  const complete = await readFile(new URL("../src/content/docs/examples/complete.mdx", import.meta.url), "utf8");
  assert.match(complete, /applicationPort/);
  assert.match(complete, /PortProtocol\.Tcp/);
  assert.match(complete, /PortPublishMode\.Ingress/);
});
