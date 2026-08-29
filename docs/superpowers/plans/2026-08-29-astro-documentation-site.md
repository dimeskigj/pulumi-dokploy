# Astro Documentation Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and deploy a polished Astro Starlight documentation site for the Pulumi Dokploy provider at `https://gjorgjidimeski.github.io/pulumi-dokploy/`.

**Architecture:** An isolated `website/` package contains the Starlight application, curated MDX, reusable Astro components, and deterministic Node-based generators. The generators consume the checked-in Pulumi schema and complete provider examples, while root Make targets and strict CI policy integrate validation and GitHub Pages deployment with the provider repository.

**Tech Stack:** Node.js 24.18.0, Astro 7.2.9, Astro Starlight 0.41.10, TypeScript 6.0.3, Sharp 0.35.4, Node's built-in test runner, GitHub Pages Actions

## Global Constraints

- Serve from `site: "https://gjorgjidimeski.github.io"` and `base: "/pulumi-dokploy"`; no code may assume a root deployment.
- Use Node.js `24.18.0`; Astro must remain compatible with its `>=22.12.0` engine floor.
- Pin `astro` to `7.2.9`, `@astrojs/starlight` to `0.41.10`, `@astrojs/check` to `0.9.10`, `typescript` to `6.0.3`, and `sharp` to `0.35.4` in the lockfile.
- Treat `provider/cmd/pulumi-resource-dokploy/schema.json` as the only API-reference source.
- Treat the tracked programs under `examples/` as the only complete-example source.
- Support TypeScript, Python, Go, C#, Java, and YAML everywhere language tabs are used.
- Keep generated reference and complete-example pages checked in and fail CI on generation drift.
- Use a Dokploy-inspired dark visual system with cyan and deployment-green accents; retain an accessible light mode.
- Do not require remote fonts, icons, graphics, analytics, server-side services, or runtime content fetches.
- Keep Pulumi Registry documentation independent; the website is the canonical expanded user guide.
- Pin GitHub Actions by full commit SHA and use only `contents: read`, `pages: write`, and `id-token: write` for Pages deployment.
- Preserve the strict exact workflow filename/job policy in `scripts/normalize_ci.py`; only the explicit Pages workflow may be added.
- Follow TDD for generator, policy, and structural behavior; run each failing test before adding its implementation.

---

### Task 1: Starlight Application Foundation

**Files:**
- Create: `website/package.json`
- Create: `website/package-lock.json`
- Create: `website/astro.config.mjs`
- Create: `website/tsconfig.json`
- Create: `website/src/content.config.ts`
- Create: `website/src/content/docs/index.mdx`
- Create: `website/src/styles/global.css`
- Create: `website/tests/config.test.mjs`
- Modify: `.mise.toml`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: repository URL and Pages URL from the approved design.
- Produces: a buildable `website/` package; Starlight docs collection `docs`; commands `dev`, `generate`, `check:generated`, `test`, `check`, and `build` used by all later tasks.

- [ ] **Step 1: Write the failing configuration test**

Create `website/tests/config.test.mjs`:

```js
import assert from "node:assert/strict";
import test from "node:test";

test("Astro is configured for the project Pages URL", async () => {
  const { default: config } = await import("../astro.config.mjs");
  assert.equal(config.site, "https://gjorgjidimeski.github.io");
  assert.equal(config.base, "/pulumi-dokploy");
  assert.equal(config.output, "static");
  assert.equal(config.integrations.length, 1);
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test website/tests/config.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `website/astro.config.mjs`.

- [ ] **Step 3: Create the pinned website package and toolchain configuration**

Create `website/package.json` with exact dependencies and commands:

```json
{
  "name": "pulumi-dokploy-docs",
  "private": true,
  "type": "module",
  "engines": { "node": ">=22.12.0" },
  "scripts": {
    "dev": "astro dev",
    "generate": "node scripts/generate-reference.mjs && node scripts/generate-examples.mjs",
    "check:generated": "npm run generate && git diff --exit-code -- src/content/docs/reference src/content/docs/examples/complete.mdx",
    "test": "node --test tests/*.test.mjs",
    "test:built": "node --test tests/site-output.built-test.mjs",
    "check": "astro check && npm test",
    "build": "astro build"
  },
  "dependencies": {
    "@astrojs/starlight": "0.41.10",
    "astro": "7.2.9",
    "sharp": "0.35.4"
  },
  "devDependencies": {
    "@astrojs/check": "0.9.10",
    "typescript": "6.0.3"
  }
}
```

Run: `npm install --prefix website --package-lock-only`

Add `node = "24.18.0"` to `.mise.toml`. Add `website/dist/` and `website/.astro/` to `.gitignore`; existing `**/node_modules/` already covers website dependencies.

- [ ] **Step 4: Create the minimal Starlight configuration and content collection**

Create `website/astro.config.mjs`:

```js
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://gjorgjidimeski.github.io",
  base: "/pulumi-dokploy",
  output: "static",
  integrations: [
    starlight({
      title: "Pulumi Dokploy",
      description: "Deploy and manage Dokploy infrastructure with Pulumi.",
      customCss: ["./src/styles/global.css"],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/gjorgjidimeski/pulumi-dokploy",
        },
      ],
      sidebar: [{ label: "Overview", link: "/" }],
    }),
  ],
});
```

Create `website/src/content.config.ts`:

```ts
import { defineCollection } from "astro:content";
import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
};
```

Create `website/tsconfig.json` extending `astro/tsconfigs/strict`, a minimal `global.css`, and `index.mdx` with `title: Pulumi Dokploy`, `description`, and one getting-started link built with `/pulumi-dokploy/` base awareness through Astro's normal link handling.

- [ ] **Step 5: Install dependencies and verify the foundation**

Run: `npm ci --prefix website`

Run: `npm --prefix website test`

Expected: PASS for the Pages URL configuration test.

Run: `npm --prefix website run check && npm --prefix website run build`

Expected: Astro check passes and `website/dist/index.html` exists.

- [ ] **Step 6: Commit the foundation**

```bash
git add .gitignore .mise.toml website
git commit -m "docs: scaffold Astro Starlight site"
```

---

### Task 2: Pulumi Schema Reference Model

**Files:**
- Create: `website/scripts/reference-model.mjs`
- Create: `website/tests/reference-model.test.mjs`

**Interfaces:**
- Consumes: Pulumi package schema JSON with `config.variables`, `types`, and `resources`.
- Produces: `loadSchema(path): Promise<object>`, `parseSchema(schema, options?): ReferenceModel`, `formatType(property): string`, and `slugFromToken(token): string`.
- `ReferenceModel` is `{ config: PropertyRow[], resources: ResourceModel[], types: TypeModel[] }`.
- `PropertyRow` is `{ name, type, typeHref, description, required, secret, replaceOnChanges, defaultValue, environment }` where `typeHref` is `string | null` and `environment` is `string[]`.
- `ResourceModel` is `{ token, name, slug, description, inputs: PropertyRow[], outputs: PropertyRow[] }`.
- `TypeModel` is `{ token, name, slug, description, properties: PropertyRow[] }`.

- [ ] **Step 1: Write failing model tests**

Create a minimal fixture inline in `website/tests/reference-model.test.mjs` and assert exact behavior:

```js
import assert from "node:assert/strict";
import test from "node:test";
import { formatType, parseSchema, slugFromToken } from "../scripts/reference-model.mjs";

const schema = {
  name: "dokploy",
  config: {
    variables: {
      apiKey: {
        type: "string",
        description: "API key.",
        secret: true,
        defaultInfo: { environment: ["DOKPLOY_API_KEY"] },
      },
    },
  },
  types: {
    "dokploy:index:Source": {
      type: "object",
      description: "Source.",
      properties: { branch: { type: "string", description: "Branch." } },
      required: ["branch"],
    },
  },
  resources: {
    "dokploy:index:Application": {
      description: "Application.",
      inputProperties: {
        environmentId: {
          type: "string",
          description: "Environment.",
          replaceOnChanges: true,
        },
        source: { $ref: "#/types/dokploy:index:Source", description: "Source." },
      },
      requiredInputs: ["environmentId", "source"],
      properties: {
        applicationId: { type: "string", description: "ID." },
      },
      required: ["applicationId"],
    },
  },
};

test("normalizes config, inputs, outputs, and complex types", () => {
  const model = parseSchema(schema, { expectedResources: new Set(["Application"]) });
  assert.deepEqual(model.config[0], {
    name: "apiKey",
    type: "string",
    typeHref: null,
    description: "API key.",
    required: false,
    secret: true,
    replaceOnChanges: false,
    defaultValue: null,
    environment: ["DOKPLOY_API_KEY"],
  });
  assert.equal(model.resources[0].slug, "application");
  assert.equal(model.resources[0].inputs[0].replaceOnChanges, true);
  assert.equal(model.resources[0].inputs[1].typeHref, "../types/#source");
  assert.equal(model.types[0].properties[0].required, true);
});

test("formats every schema type used by the provider", () => {
  assert.equal(formatType({ type: "string" }), "string");
  assert.equal(formatType({ type: "integer" }), "integer");
  assert.equal(formatType({ type: "boolean" }), "boolean");
  assert.equal(formatType({ type: "array", items: { type: "string" } }), "string[]");
  assert.equal(formatType({ $ref: "#/types/dokploy:index:Source" }), "Source");
});

test("derives stable lowercase slugs", () => {
  assert.equal(slugFromToken("dokploy:index:Postgres"), "postgres");
});
```

- [ ] **Step 2: Run the model tests to verify they fail**

Run: `node --test website/tests/reference-model.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `reference-model.mjs`.

- [ ] **Step 3: Implement strict schema parsing**

Implement `reference-model.mjs` with these validations:

```js
const EXPECTED_RESOURCES = new Set([
  "Application", "Compose", "Domain", "Environment", "Postgres", "Project", "Redis",
]);

export function slugFromToken(token) {
  const name = token.split(":").at(-1);
  if (!name) throw new Error(`Invalid Pulumi token: ${token}`);
  return name.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

export function formatType(property) {
  if (property.$ref) return property.$ref.split(":").at(-1);
  if (property.type === "array" && property.items) return `${formatType(property.items)}[]`;
  if (["string", "integer", "number", "boolean"].includes(property.type)) return property.type;
  throw new Error(`Unsupported Pulumi property type: ${JSON.stringify(property)}`);
}
```

Add a `normalizeProperties(properties, requiredNames, context)` helper that sorts property names, requires non-empty descriptions, carries booleans with `=== true`, preserves scalar defaults, and copies `defaultInfo.environment ?? []`. `parseSchema` must reject a package name other than `dokploy`, an unexpected resource set, an invalid resource token, missing resource/type descriptions, and dangling `$ref` values. `loadSchema` must include the source path in JSON parse and validation errors.

For referenced types, set `typeHref` with `../types/#${slugFromToken(referenceToken)}` so `dokploy:index:Source` becomes `../types/#source`; scalar and array properties use `null`. `parseSchema` defaults `options.expectedResources` to `EXPECTED_RESOURCES`; the injected set exists only so focused fixtures can validate one resource while the real schema always validates all seven.

- [ ] **Step 4: Run focused and real-schema tests**

Add a test that loads `../../provider/cmd/pulumi-resource-dokploy/schema.json`, asserts seven resources, asserts `apiKey.secret === true`, and asserts `Application.environmentId.replaceOnChanges === true`.

Run: `node --test website/tests/reference-model.test.mjs`

Expected: PASS.

- [ ] **Step 5: Commit the schema model**

```bash
git add website/scripts/reference-model.mjs website/tests/reference-model.test.mjs
git commit -m "docs: model Pulumi schema reference"
```

---

### Task 3: Deterministic Reference Page Generation

**Files:**
- Create: `website/scripts/render-reference.mjs`
- Create: `website/scripts/generate-reference.mjs`
- Create: `website/tests/render-reference.test.mjs`
- Generate: `website/src/content/docs/reference/configuration.mdx`
- Generate: `website/src/content/docs/reference/types.mdx`
- Generate: `website/src/content/docs/reference/application.mdx`
- Generate: `website/src/content/docs/reference/compose.mdx`
- Generate: `website/src/content/docs/reference/domain.mdx`
- Generate: `website/src/content/docs/reference/environment.mdx`
- Generate: `website/src/content/docs/reference/postgres.mdx`
- Generate: `website/src/content/docs/reference/project.mdx`
- Generate: `website/src/content/docs/reference/redis.mdx`

**Interfaces:**
- Consumes: `ReferenceModel` from `parseSchema` in Task 2.
- Produces: `renderConfiguration(model): string`, `renderTypes(model): string`, `renderResource(resource): string`, and `replaceGeneratedDirectory(target, files, write?): Promise<void>` where `write` defaults to `fs.writeFile` and is injectable only for failure-path tests.
- Generated MDX imports `PropertyTable` from `../../../components/PropertyTable.astro`; Task 4 provides that component before the first full site build.

- [ ] **Step 1: Write failing renderer tests**

Create `website/tests/render-reference.test.mjs` with a one-resource model and exact assertions:

```js
import assert from "node:assert/strict";
import test from "node:test";
import { renderResource, replaceGeneratedDirectory } from "../scripts/render-reference.mjs";

test("renders generated notice and input/output metadata", () => {
  const mdx = renderResource({
    token: "dokploy:index:Application",
    name: "Application",
    slug: "application",
    description: "A Dokploy application.",
    inputs: [{
      name: "environmentId", type: "string", typeHref: null, description: "Environment.",
      required: true, secret: false, replaceOnChanges: true,
      defaultValue: null, environment: [],
    }],
    outputs: [],
  });
  assert.match(mdx, /title: Application/);
  assert.match(mdx, /Generated from `schema\.json`/);
  assert.match(mdx, /replaceOnChanges: true/);
});
```

Add a temporary-directory test proving `replaceGeneratedDirectory` leaves an existing target unchanged when a supplied write callback throws, then replaces it completely on success.

- [ ] **Step 2: Run renderer tests to verify they fail**

Run: `node --test website/tests/render-reference.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `render-reference.mjs`.

- [ ] **Step 3: Implement MDX rendering and atomic replacement**

Use JSON serialization for component props so quotes and newlines cannot produce malformed MDX:

```js
function frontmatter(title, description) {
  return `---\ntitle: ${JSON.stringify(title)}\ndescription: ${JSON.stringify(description)}\n---\n`;
}

export function renderResource(resource) {
  return `${frontmatter(resource.name, resource.description)}
import PropertyTable from "../../../components/PropertyTable.astro";

> Generated from \`schema.json\`. Add lifecycle guidance to curated guides, not this file.

${resource.description}

## Inputs

<PropertyTable properties={${JSON.stringify(resource.inputs)}} />

## Outputs

<PropertyTable properties={${JSON.stringify(resource.outputs)}} />
`;
}
```

Implement the configuration page, one `types.mdx` page with stable anchors for every complex type, and resource pages sorted by resource name. Write all files under a sibling temporary directory created with `fs.mkdtemp`. If the target exists, rename it to a unique sibling backup, rename the complete temporary directory to the target, then remove the backup. If the second rename fails, restore the backup before rethrowing. Clean temporary and backup paths in `finally` without masking the original error.

- [ ] **Step 4: Implement the generator entry point**

`website/scripts/generate-reference.mjs` resolves paths relative to `import.meta.url`, loads `../../provider/cmd/pulumi-resource-dokploy/schema.json`, parses it, creates a filename-to-content map, and calls `replaceGeneratedDirectory`. On failure it calls `console.error("Reference generation failed: " + error.message)` and sets `process.exitCode = 1`.

- [ ] **Step 5: Run tests and generate tracked reference pages**

Run: `node --test website/tests/render-reference.test.mjs`

Expected: PASS.

Run: `node website/scripts/generate-reference.mjs`

Expected: reference generation succeeds without depending on the complete-example generator added in Task 6.

Run: `test "$(ls website/src/content/docs/reference/*.mdx | wc -l)" -eq 9`

Expected: PASS for configuration, types, and seven resources.

- [ ] **Step 6: Commit the generator and generated pages**

```bash
git add website/scripts website/tests website/src/content/docs/reference
git commit -m "docs: generate provider API reference"
```

---

### Task 4: Documentation Components And Visual System

**Files:**
- Create: `website/src/components/PropertyTable.astro`
- Create: `website/src/components/LanguageTabs.astro`
- Create: `website/src/components/ResourceSummary.astro`
- Create: `website/src/components/LifecycleNote.astro`
- Create: `website/src/components/RelatedResources.astro`
- Create: `website/src/components/HomeHero.astro`
- Create: `website/src/components/CapabilityMap.astro`
- Replace: `website/src/styles/global.css`
- Create: `website/tests/components.test.mjs`

**Interfaces:**
- Consumes: `PropertyRow[]` JSON emitted by Task 3 and `{ language, label, code }[]` example arrays emitted by Task 6.
- Produces: static, accessible HTML components used by generated and curated MDX.
- `PropertyTable` props: `{ properties: PropertyRow[] }`.
- `LanguageTabs` props: `{ examples: { language: string, label: string, code: string }[] }` and requires exactly `typescript`, `python`, `go`, `csharp`, `java`, `yaml` in that order.
- `ResourceSummary` props: `{ name: string, description: string, lifecycle: string, related: { label: string, href: string }[] }`.

- [ ] **Step 1: Write failing source-structure tests**

Create `website/tests/components.test.mjs`:

```js
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("property metadata has explicit accessible labels", async () => {
  const source = await readFile(new URL("../src/components/PropertyTable.astro", import.meta.url), "utf8");
  for (const label of ["Required", "Secret", "Replaces resource"]) {
    assert.match(source, new RegExp(label));
  }
});

test("language tabs enforce all supported languages", async () => {
  const source = await readFile(new URL("../src/components/LanguageTabs.astro", import.meta.url), "utf8");
  for (const language of ["typescript", "python", "go", "csharp", "java", "yaml"]) {
    assert.match(source, new RegExp(`"${language}"`));
  }
  assert.match(source, /syncKey="language"/);
});
```

- [ ] **Step 2: Run component tests to verify they fail**

Run: `node --test website/tests/components.test.mjs`

Expected: FAIL with `ENOENT` for the first missing component.

- [ ] **Step 3: Implement accessible reusable components**

Use Starlight's `Tabs`, `TabItem`, and `Code` exports in `LanguageTabs.astro`:

```astro
---
import { Code, TabItem, Tabs } from "@astrojs/starlight/components";

const ORDER = ["typescript", "python", "go", "csharp", "java", "yaml"];
const { examples } = Astro.props;
if (examples.map((example) => example.language).join(",") !== ORDER.join(",")) {
  throw new Error(`LanguageTabs requires: ${ORDER.join(", ")}`);
}
---

<Tabs syncKey="language">
  {examples.map((example) => (
    <TabItem label={example.label}>
      <Code code={example.code} lang={example.language} />
    </TabItem>
  ))}
</Tabs>
```

Render `PropertyTable` as a real table with visible property names, code-formatted types/defaults, links for non-null `typeHref`, and text badges for Required, Secret, and Replaces resource. Do not communicate metadata by color alone. Implement the remaining components as semantic sections and lists; links receive a visible keyboard focus style.

- [ ] **Step 4: Implement the dark-first theme**

Define Starlight CSS variables for near-black navy surfaces, cyan interaction, deployment green success, high-contrast text, border layers, and restrained shadows. Add a local CSS grid texture to the splash hero using gradients, not an image. Include:

```css
:root {
  --sl-color-accent-low: #062d35;
  --sl-color-accent: #22d3ee;
  --sl-color-accent-high: #b8f7ff;
  --sl-color-bg: #f7fbfc;
  --dokploy-success: #2ee59d;
}

:root[data-theme="dark"] {
  --sl-color-bg: #071014;
  --sl-color-bg-nav: rgba(7, 16, 20, 0.9);
  --sl-color-bg-sidebar: #09151a;
  --sl-color-hairline: #183039;
  --sl-color-text: #d9e7eb;
  --dokploy-success: #43f0a8;
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition: none !important; }
}
```

Use fluid type/spacing, a maximum readable prose width, mobile table overflow, clear `:focus-visible`, and no remote `@font-face` or `url(https://...)` declarations.

- [ ] **Step 5: Verify components and generated reference integration**

Run: `node --test website/tests/components.test.mjs`

Expected: PASS.

Run: `npm --prefix website run check && npm --prefix website run build`

Expected: generated reference MDX compiles with `PropertyTable`; no Astro diagnostics.

- [ ] **Step 6: Commit components and theme**

```bash
git add website/src/components website/src/styles website/tests/components.test.mjs
git commit -m "docs: add Dokploy documentation design system"
```

---

### Task 5: Landing Page And Curated Guides

**Files:**
- Replace: `website/src/content/docs/index.mdx`
- Create: `website/src/content/docs/getting-started/installation.mdx`
- Create: `website/src/content/docs/getting-started/first-deployment.mdx`
- Create: `website/src/content/docs/concepts/projects-and-environments.mdx`
- Create: `website/src/content/docs/concepts/sources.mdx`
- Create: `website/src/content/docs/concepts/lifecycle-and-state.mdx`
- Create: `website/src/content/docs/concepts/secrets.mdx`
- Create: `website/src/content/docs/guides/applications.mdx`
- Create: `website/src/content/docs/guides/compose.mdx`
- Create: `website/src/content/docs/guides/databases.mdx`
- Create: `website/src/content/docs/guides/domains.mdx`
- Create: `website/src/content/docs/guides/imports.mdx`
- Create: `website/src/content/docs/guides/troubleshooting.mdx`
- Create: `website/src/content/docs/contributing.mdx`
- Modify: `website/astro.config.mjs`
- Create: `website/tests/content.test.mjs`

**Interfaces:**
- Consumes: reusable components from Task 4 and behavior documented in `README.md`, the provider design spec, and the schema.
- Produces: canonical conceptual documentation and complete Starlight sidebar navigation.

- [ ] **Step 1: Write the failing content inventory test**

Create `website/tests/content.test.mjs` with an exact required-file map and required safety statements:

```js
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
```

- [ ] **Step 2: Run the content tests to verify they fail**

Run: `node --test website/tests/content.test.mjs`

Expected: FAIL on the first missing curated page.

- [ ] **Step 3: Build the custom landing page**

Set `template: splash` and `pagefind: false` on the home page. Compose `HomeHero` and `CapabilityMap` with this exact hierarchy: "Deploy Dokploy with Pulumi", a two-sentence value proposition, "Get started" and "Resource reference" actions, a compact TypeScript program using `Project` and `Application`, seven capability cards, the six-language strip, lifecycle assurances, a GitHub link, and a `https://www.pulumi.com/registry/packages/dokploy/` Registry link. Do not claim a release exists; phrase Registry availability as beginning after the first release is published.

- [ ] **Step 4: Write focused conceptual and task-oriented guides**

Each page must include actionable commands or provider examples and cross-links:

- Installation: six package installation commands and the provider binary relationship.
- First deployment: endpoint/API-key secret configuration, `pulumi up`, observed deployment completion, and `pulumi destroy`.
- Projects/environments: default production ownership and explicit non-production environments.
- Sources: Application Docker/public Git/existing GitLab and Compose raw/Git/existing GitLab discriminators.
- Lifecycle/state: previews, replacement fields, deployment polling, import/refresh, partial state, and not-found behavior.
- Secrets: all secret categories, `pulumi config set --secret`, derived secret outputs, and never-logged guarantees.
- Resource guides: source-specific creation, update behavior, relevant dependencies, and links to generated references.
- Compose guide: `deleteVolumesOnDestroy` defaults to `false` with an explicit destructive warning.
- Imports: `pulumi import dokploy:index:Project existing p1`, refresh review, and write-only secret restoration.
- Troubleshooting: endpoint normalization, authorization, failed deployments with retained IDs, unsupported source transitions, and missing imported passwords.
- Contributing: `mise install`, provider checks, `make docs_generate`, `make docs_check`, generated-file ownership, and Pages workflow.

- [ ] **Step 5: Configure the complete sidebar**

Replace the initial sidebar with explicit groups for Get Started, Core Concepts, Guides, Resources, Examples, and Contributing. Resource links must be listed explicitly in the order Project, Environment, Application, Compose, Postgres, Redis, Domain, then Configuration and Complex Types. Do not rely on filesystem autogeneration for canonical ordering.

- [ ] **Step 6: Verify curated content**

Run: `node --test website/tests/content.test.mjs`

Expected: PASS.

Run: `npm --prefix website run check && npm --prefix website run build`

Expected: PASS with no broken internal content references reported by Astro/Starlight.

- [ ] **Step 7: Commit landing page and guides**

```bash
git add website/astro.config.mjs website/src/content/docs website/tests/content.test.mjs
git commit -m "docs: add provider guides and landing page"
```

---

### Task 6: Six-Language Complete Examples And Repository Entry Point

**Files:**
- Create: `website/scripts/generate-examples.mjs`
- Create: `website/tests/generate-examples.test.mjs`
- Create: `website/tests/site-output.built-test.mjs`
- Generate: `website/src/content/docs/examples/complete.mdx`
- Create: `website/src/content/docs/examples/index.mdx`
- Modify: `README.md`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `examples/nodejs/index.ts`, `examples/python/__main__.py`, `examples/go/main.go`, `examples/dotnet/Program.cs`, `examples/java/src/main/java/generated_program/App.java`, and `examples/yaml/Pulumi.yaml`.
- Produces: `loadExamples(root): Promise<Example[]>`, `renderCompleteExamples(examples): string`, checked-in `complete.mdx`, and root commands `docs_generate`, `docs_check`, `docs_build`.
- `Example` is `{ language, label, code, sourcePath }`, ordered TypeScript, Python, Go, C#, Java, YAML.

- [ ] **Step 1: Write failing complete-example tests**

Create `website/tests/generate-examples.test.mjs`:

```js
import assert from "node:assert/strict";
import test from "node:test";
import { loadExamples, renderCompleteExamples } from "../scripts/generate-examples.mjs";

test("loads all complete examples in canonical order", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  assert.deepEqual(examples.map(({ language }) => language), [
    "typescript", "python", "go", "csharp", "java", "yaml",
  ]);
  assert.ok(examples.every(({ code }) => code.endsWith("\n")));
});

test("renders one synchronized six-language component", async () => {
  const examples = await loadExamples(new URL("../../examples/", import.meta.url));
  const mdx = renderCompleteExamples(examples);
  assert.equal((mdx.match(/language:/g) ?? []).length, 6);
  assert.match(mdx, /<LanguageTabs examples=\{examples\} \/>/);
  assert.doesNotMatch(mdx, /dokploy\.example\.com|x-api-key:/);
});
```

- [ ] **Step 2: Run the example tests to verify they fail**

Run: `node --test website/tests/generate-examples.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `generate-examples.mjs`.

- [ ] **Step 3: Implement deterministic complete-example generation**

Map the six exact paths from the Interfaces section. Normalize CRLF to LF and require a final newline. Reject a missing/empty source, strings matching `https://dokploy.example.com`, PEM/private-key headers, or an `x-api-key:` literal. Render frontmatter, import `LanguageTabs`, export a JSON-safe `examples` array in MDX, explain that the page is generated from tracked examples, and render `<LanguageTabs examples={examples} />`. Write through a sibling temporary file and rename on success.

- [ ] **Step 4: Add the curated examples index and generate output**

The examples index explains the canonical YAML source and Pulumi conversion into five SDK languages, links to Complete Example, and links to the provider repository directories. It must state that placeholders are intentionally invalid and secrets must be supplied with secret configuration.

Run: `npm --prefix website run generate`

Expected: both reference and complete-example generation succeed.

Run: `npm --prefix website run check:generated`

Expected: PASS with no diff after the second generation.

- [ ] **Step 5: Add root documentation commands**

Extend `.PHONY` and add:

```make
docs_generate:
	npm ci --prefix website
	npm --prefix website run generate

docs_check:
	npm ci --prefix website
	npm --prefix website run check:generated
	npm --prefix website run check
	npm --prefix website run build
	npm --prefix website run test:built

docs_build:
	npm ci --prefix website
	npm --prefix website run build
```

- [ ] **Step 6: Make the README a concise site entry point**

Keep the existing title, build badge, provider summary, package installation table, endpoint/API-key secret commands, seven-resource list, one import command, and development commands. Add a prominent documentation link immediately after the summary. Replace detailed lifecycle/source prose with a short capabilities paragraph and links to the site's Get Started, Resources, and Guides pages using absolute Pages URLs.

- [ ] **Step 7: Add built-site link, base-path, and search validation**

Create `website/tests/site-output.built-test.mjs` using only `node:fs/promises`, `node:path`, `node:assert/strict`, and `node:test`. Implement `collectFiles(directory)` recursively and `candidateSiteTargets(dist, sourceFile, rawUrl)` with these exact rules:

```js
const BASE = "/pulumi-dokploy";
const ORIGIN = "https://gjorgjidimeski.github.io";

function candidateSiteTargets(dist, sourceFile, rawUrl) {
  const sourceRoute = path.relative(dist, sourceFile)
    .replaceAll(path.sep, "/")
    .replace(/index\.html$/, "");
  const url = new URL(rawUrl, `${ORIGIN}${BASE}/${sourceRoute}`);
  if (url.origin !== ORIGIN) return [];
  assert.ok(url.pathname === BASE || url.pathname.startsWith(`${BASE}/`), `URL escapes base: ${rawUrl}`);
  const relative = decodeURIComponent(url.pathname.slice(BASE.length)).replace(/^\//, "");
  const target = path.join(dist, relative);
  return relative.endsWith("/") || relative === ""
    ? [path.join(target, "index.html")]
    : [target, path.join(target, "index.html")];
}
```

For every built HTML file, extract quoted `href` and `src` values, ignore `#`, `mailto:`, `tel:`, and non-site origins, and assert at least one candidate target exists. For links with a hash, load the existing HTML candidate and assert it contains the decoded `id`. Also assert these files/directories exist: `index.html`, `reference/application/index.html`, `examples/complete/index.html`, and `pagefind/`. This makes broken internal links, root-based URLs, missing key routes, and absent static search fail `test:built`.

- [ ] **Step 8: Verify examples, root commands, README, and built output**

Run: `node --test website/tests/generate-examples.test.mjs`

Expected: PASS.

Run: `make docs_check`

Expected: generation is clean, tests/check pass, and production build succeeds.

- [ ] **Step 9: Commit examples and repository integration**

```bash
git add Makefile README.md website/package.json website/package-lock.json website/scripts/generate-examples.mjs website/tests/generate-examples.test.mjs website/tests/site-output.built-test.mjs website/src/content/docs/examples
git commit -m "docs: publish six-language provider examples"
```

---

### Task 7: GitHub Pages Deployment And Strict CI Policy

**Files:**
- Create: `.github/workflows/pages.yml`
- Modify: `scripts/normalize_ci.py`
- Modify: `scripts/test_normalize_ci.py`
- Modify: `.github/workflows/build.yml` through the normalizer's deterministic insertion

**Interfaces:**
- Consumes: `make docs_check`, `website/package-lock.json`, and `website/dist` from earlier tasks.
- Produces: validated jobs `build` and `deploy` in `pages.yml`; a PR documentation gate in generated `build.yml`; exact workflow allowlist support for `pages.yml`.

- [ ] **Step 1: Write failing workflow-policy tests**

Add to `WorkflowPolicyTests`:

```python
def test_pages_workflow_is_explicitly_allowed_with_two_non_go_jobs(self):
    self.assertIn("pages.yml", NORMALIZE.GENERATED_WORKFLOW_NAMES)
    self.assertEqual(
        NORMALIZE.WORKFLOW_JOB_POLICY["pages.yml"],
        {"build": False, "deploy": False},
    )
    fixture = """jobs:
  build:
    steps:
    - run: npm ci --prefix website
  deploy:
    steps:
    - uses: actions/deploy-pages@sha
"""
    NORMALIZE.validate_workflow_jobs(
        "pages.yml", fixture, {"build": False, "deploy": False}
    )
```

Add a gate test asserting `"make docs_check" in NORMALIZE.EXPECTED_BUILD_GATES`.

- [ ] **Step 2: Run policy tests to verify they fail**

Run: `python3 -m unittest scripts/test_normalize_ci.py -v`

Expected: FAIL because `pages.yml` and `make docs_check` are not policy members.

- [ ] **Step 3: Extend the exact CI policy and deterministic build normalization**

Add `pages.yml` to `GENERATED_WORKFLOW_NAMES`, add `{"build": False, "deploy": False}` to `WORKFLOW_JOB_POLICY`, and add `make docs_check` to `EXPECTED_BUILD_GATES`.

In `main()`, use `insert_in_job` to insert this exact block before the existing `Check OpenAPI and generated SDKs` step in the `prerequisites` job:

```yaml
    - name: Setup Node for docs
      uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38 # v6
      with:
        node-version: 24.18.0
        cache: npm
        cache-dependency-path: website/package-lock.json
    - name: Check documentation
      run: make docs_check
```

The insertion must be idempotent and scoped to `prerequisites`; a fixture test must prove a missing or duplicate marker fails.

- [ ] **Step 4: Create the pinned Pages workflow**

Create `.github/workflows/pages.yml` with push-to-`main` and manual triggers, Pages concurrency, and these pinned actions:

```yaml
name: pages
on:
  push:
    branches: [main]
  workflow_dispatch: {}

permissions:
  contents: read

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
    - uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38 # v6
      with:
        node-version: 24.18.0
        cache: npm
        cache-dependency-path: website/package-lock.json
    - uses: actions/configure-pages@983d7736d9b0ae728b81ab479565c72886d7745b # v5
    - run: npm ci --prefix website
    - run: npm --prefix website run build
    - uses: actions/upload-pages-artifact@7b1f4a764d45c48632c6b24a0339c27f5614fb0b # v4
      with:
        path: website/dist
  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    needs: build
    permissions:
      pages: write
      id-token: write
    runs-on: ubuntu-latest
    steps:
    - id: deployment
      uses: actions/deploy-pages@d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e # v4
```

- [ ] **Step 5: Regenerate CI and run policy tests**

Run: `make ci-mgmt`

Expected: generated workflows normalize, `build.yml` contains exactly one executable `make docs_check`, and `pages.yml` remains present.

Run: `python3 -m unittest scripts/test_normalize_ci.py -v`

Expected: PASS.

Run: `python3 scripts/normalize_ci.py`

Expected: PASS and no second-run workflow diff.

- [ ] **Step 6: Commit Pages deployment and policy**

```bash
git add .github/workflows/build.yml .github/workflows/pages.yml scripts/normalize_ci.py scripts/test_normalize_ci.py
git commit -m "ci: deploy documentation to GitHub Pages"
```

---

### Task 8: End-To-End Documentation Verification

**Files:**
- Modify only files required to resolve failures found by the commands below.

**Interfaces:**
- Consumes: all prior documentation tasks.
- Produces: a clean, reproducible branch ready for review and GitHub Pages repository configuration.

- [ ] **Step 1: Verify deterministic generation and website quality**

Run: `make docs_check`

Expected: reference/example generation is unchanged; Node tests, Astro check, and production build pass.

- [ ] **Step 2: Verify generated output at the project base path**

Run: `test -f website/dist/index.html && test -f website/dist/reference/application/index.html && test -f website/dist/examples/complete/index.html`

Expected: PASS.

Run: `! rg -n '(?:href|src)="/(?!pulumi-dokploy(?:/|"))' website/dist --glob '*.html' --pcre2`

Expected: no internal site link or asset begins at `/` without the `/pulumi-dokploy/` prefix; protocol-relative and external URLs are reviewed separately.

- [ ] **Step 3: Re-run repository safety gates affected by documentation changes**

Run: `python3 -m unittest scripts/test_normalize_ci.py -v`

Expected: PASS.

Run: `mise exec -- make lint`

Expected: PASS.

Run: `git diff --check`

Expected: PASS.

- [ ] **Step 4: Verify repository cleanliness after generated build artifacts are ignored**

Run: `git status --short`

Expected: no untracked `website/dist`, `.astro`, or `node_modules`; only intentional source changes, if a preceding fix was necessary.

- [ ] **Step 5: Commit any verification fixes**

If Step 1-4 required changes, stage only those files and commit:

```bash
git add .gitignore .mise.toml Makefile README.md website .github/workflows/build.yml .github/workflows/pages.yml scripts/normalize_ci.py scripts/test_normalize_ci.py
git commit -m "docs: close documentation verification gaps"
```

If no files changed, do not create an empty commit.

- [ ] **Step 6: Record the external GitHub setting required after merge**

In the final implementation report, state exactly: "Set repository Settings > Pages > Build and deployment > Source to GitHub Actions." This is the only required manual deployment setting; no custom domain or `CNAME` is used.
