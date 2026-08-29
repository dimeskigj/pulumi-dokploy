# Astro Documentation Site Design

## Summary

Build a polished documentation site for the Pulumi Dokploy provider with Astro
and the official Starlight documentation template. The site is the canonical
user documentation, combines curated guides with schema-generated API
reference pages, and deploys to GitHub Pages at:

`https://gjorgjidimeski.github.io/pulumi-dokploy/`

The experience uses a Dokploy-inspired dark visual language while preserving
Starlight's accessibility, navigation, search, responsive behavior, and
content conventions.

## Goals

- Give new users a clear path from installation to a working Dokploy
  deployment.
- Document all seven provider resources and provider configuration from the
  checked-in Pulumi schema.
- Provide examples for TypeScript, Python, Go, C#, Java, and YAML.
- Explain lifecycle behavior, imports, secrets, replacements, and partial
  state in practical terms.
- Make documentation changes reproducible and validate them in CI.
- Deploy the static site automatically from `main` with GitHub Pages.
- Keep the site useful on desktop and mobile and accessible with keyboard and
  assistive technology.

## Non-Goals

- Host versioned documentation for multiple provider major versions.
- Replace Pulumi Registry package documentation.
- Add a server-side application, database, authentication, analytics, or
  external search service.
- Fetch schema, examples, fonts, icons, or other required content at runtime.
- Document Dokploy APIs that the provider does not expose.
- Change provider behavior or resource schemas as part of the site work.

## Chosen Approach

Use Astro Starlight with a custom landing page, custom theme, curated MDX
content, and a small build-time schema generator.

Starlight is preferred over a custom Astro documentation shell because it
already provides accessible navigation, static search, table of contents,
syntax highlighting, Markdown and MDX content, theme controls, and responsive
layouts. The custom work can therefore focus on provider-specific content,
reference generation, and a distinctive visual identity instead of rebuilding
documentation infrastructure.

## Repository Structure

The site lives in an isolated `website/` workspace:

```text
website/
  astro.config.mjs
  package.json
  package-lock.json
  tsconfig.json
  public/
  scripts/
    generate-reference.mjs
  src/
    assets/
    components/
    content/
      docs/
        index.mdx
        getting-started/
        concepts/
        guides/
        resources/
        examples/
        reference/
        contributing.mdx
      config.ts
    styles/
  tests/
```

The root provider schema remains at
`provider/cmd/pulumi-resource-dokploy/schema.json`. Existing programs under
`examples/` remain the source of truth for complete examples.

The website has its own Node dependency lockfile so documentation tooling does
not affect generated provider SDK dependencies.

## Astro And GitHub Pages Configuration

Astro is configured with:

- `site`: `https://gjorgjidimeski.github.io`
- `base`: `/pulumi-dokploy`
- static output suitable for GitHub Pages
- Starlight as the documentation integration

All internal links, local assets, canonical URLs, social metadata, and custom
components must respect the configured base path. Components must not assume
the site is served from `/`.

The site uses no required remote runtime assets. Fonts, icons, and decorative
graphics are local or generated at build time.

## Information Architecture

The primary navigation is:

1. **Overview**: provider value proposition, capabilities, and support status.
2. **Get Started**: installation, configuration, first deployment, and destroy.
3. **Core Concepts**: projects and environments, sources, deployment
   lifecycle, state, secrets, and replacement behavior.
4. **Guides**: applications, Compose stacks, databases, domains, imports, and
   troubleshooting.
5. **Resources**: generated API reference for Project, Environment,
   Application, Compose, Postgres, Redis, and Domain.
6. **Examples**: complete examples and language-specific snippets.
7. **Configuration And Imports**: provider settings, environment variables,
   secret handling, and adoption of existing resources.
8. **Contributing**: local setup, generation, validation, and release-relevant
   documentation workflow.

The root `README.md` becomes a concise repository entry point containing the
provider summary, installation table, minimal configuration and usage, key
development commands, and a prominent link to the site. Detailed guidance is
canonical on the site rather than duplicated in the README.

## Landing Page

The custom landing page presents a stronger visual hierarchy than standard
documentation pages while remaining part of the Starlight application. It
contains:

- a direct "Deploy Dokploy with Pulumi" headline and short explanation;
- primary actions for getting started and opening the resource reference;
- a compact, syntax-highlighted Pulumi program preview;
- a capability map for the seven resources;
- a strip showing all six supported authoring languages;
- concise guarantees around previews, deployment waiting, imports, partial
  state, and secret propagation;
- links to GitHub and the Pulumi Registry package location.

The page should communicate that this is infrastructure tooling, not a generic
marketing site. Decoration supports navigation and comprehension rather than
obscuring content.

## Visual System

The primary presentation is a Dokploy-inspired dark theme:

- near-black navy backgrounds and layered dark surfaces;
- cyan for navigation and interactive emphasis;
- deployment green for success and lifecycle indicators;
- restrained glows, fine grid textures, and subtle borders;
- high-contrast body text and a compact technical typographic hierarchy.

Light mode remains fully supported. Documentation pages are visually calmer
than the landing page, with readable line lengths, predictable spacing, a
sticky table of contents where space permits, and minimal decorative effects.

The implementation customizes Starlight through documented CSS variables,
component overrides, and scoped components rather than forking Starlight.
Responsive behavior must work at mobile, tablet, and desktop widths.

## Reusable Components

Provider-specific MDX components include:

- **LanguageTabs**: synchronized tabs for TypeScript, Python, Go, C#, Java,
  and YAML examples;
- **ResourceSummary**: purpose, lifecycle behavior, and related resources;
- **PropertyTable**: inputs or outputs with type, requirement, default, and
  description;
- **SecretBadge**: identifies secret inputs and outputs;
- **ReplacementBadge**: identifies replacement-only properties;
- **LifecycleNote**: highlights deployment, import, refresh, or partial-state
  behavior;
- **RelatedResources**: links between dependent resource pages.

Components must render meaningful static HTML, support keyboard navigation,
and retain useful content if client-side JavaScript is unavailable. Client-side
hydration is used only where interaction requires it.

## Multi-Language Examples

Quickstarts and resource examples expose TypeScript, Python, Go, C#, Java, and
YAML. Complete examples already generated under `examples/` remain canonical.

Full-program language tabs read the tracked source files under `examples/`
during generation and render them without maintaining a second copy. Shorter
conceptual snippets are maintained directly in MDX and must cover all six
languages. Resource pages may link to the full generated programs instead of
repeating a large program inline.

Secret placeholders remain visibly fake and must never include real endpoints,
credentials, or state.

## Schema-Generated Reference

`website/scripts/generate-reference.mjs` reads the checked-in Pulumi schema and
produces:

- one provider configuration reference page;
- one page for each of the seven resources;
- property tables for inputs and outputs;
- requirement, default, secret, and replacement metadata;
- links to referenced complex types;
- a short notice that the page is generated from the provider schema.

Generation validates the package name, expected resource token shape, required
descriptions, supported property forms, and internal type references. It exits
with an actionable error when the schema is malformed or contains a property
shape the renderer cannot represent.

Generation writes to a temporary directory first. The generated reference
directory is replaced only after all pages are produced successfully, avoiding
partial output after a failure.

Generated files are checked into Git for review and reliable local browsing.
A drift check regenerates them and fails if `git diff` reports changes.
Curated lifecycle explanations and examples live outside generated files so
regeneration never overwrites them.

## Build And Developer Workflow

The website exposes these scripts:

- `npm run dev`: local Starlight development server;
- `npm run generate`: regenerate schema reference and full-program examples;
- `npm run check:generated`: regenerate and fail on tracked drift;
- `npm run check`: run Astro content and type checks plus focused tests;
- `npm run build`: produce the production static site;
- `npm run test`: run generator and site-structure tests.

Root Make targets provide repository-standard entry points:

- `make docs_generate`
- `make docs_check`
- `make docs_build`

`docs_check` includes generation drift detection, Astro checks, tests, and a
production build. Existing provider checks remain unchanged except for adding
the documentation check to CI.

## Error Handling

Documentation generation errors identify the schema token or property that
caused the failure and explain the unsupported or missing value. Build errors
must propagate with a non-zero status; workflows do not suppress them.

Broken internal links, missing required pages, invalid frontmatter, malformed
MDX, inaccessible language-tab markup, and base-path errors fail validation.
External links are not required for every local build because network access
is unreliable, but a separate non-blocking or scheduled link check may be
added later.

GitHub Pages deployment occurs only after a successful production build. A
failed deployment leaves the last successful Pages artifact available.

## Testing And Validation

Focused tests cover:

- schema conversion helpers and supported Pulumi type shapes;
- required configuration and seven-resource page generation;
- secret, default, required, and replacement metadata;
- deterministic generation and safe replacement of output;
- all six language tabs and their accessible tab relationships;
- navigation to required curated and generated pages;
- internal links and asset URLs under `/pulumi-dokploy`;
- production generation of key pages and static search assets;
- responsive smoke checks for the custom landing page where practical.

`astro check` validates TypeScript, content collections, and component use.
The production build is the final integration check for routing, MDX, and
static assets.

## Continuous Integration

Pull requests run `make docs_check` but never deploy. The existing generated CI
workflow policy has an exact filename and job allowlist, so documentation work
must update `scripts/normalize_ci.py` and its tests to recognize the Pages
workflow explicitly. The policy continues to reject unknown workflow files and
unexpected jobs.

The Pages workflow uses pinned official GitHub actions and minimal permissions:

- `contents: read`
- `pages: write`
- `id-token: write`

It builds on pushes to `main` and supports manual dispatch. It checks out the
repository, installs the locked Node dependencies under `website/`, runs the
production documentation build, uploads `website/dist`, and deploys through
GitHub Pages. Concurrency allows a newer deployment to replace an older queued
deployment without allowing simultaneous Pages writes.

The repository must be configured in GitHub to use GitHub Actions as its Pages
source. No custom domain or `CNAME` file is required.

## Documentation Ownership

The site is the canonical user documentation. Changes to provider inputs,
outputs, defaults, secrets, or replacement semantics require schema
regeneration and any corresponding curated guide updates. CI catches generated
reference drift; review remains responsible for conceptual accuracy.

The root README links to the deployed site and intentionally remains concise.
Pulumi Registry docs continue to derive from the provider schema independently.

## Acceptance Criteria

- `website/` builds from a clean checkout with locked dependencies.
- The production site works at the `/pulumi-dokploy/` base path.
- The landing page has the approved Dokploy-inspired visual direction and is
  responsive in dark and light modes.
- Curated getting-started, concepts, guide, example, import, and contributing
  pages are present and navigable.
- Configuration and all seven resource references are generated from the
  checked-in provider schema.
- Quickstarts and examples expose all six supported languages.
- Search indexes both curated and generated documentation.
- The root README points users to the site and avoids duplicating detailed
  guidance.
- CI validates generation drift, Astro content and types, tests, and a
  production build.
- The strict workflow policy accepts the explicitly validated Pages workflow
  and continues rejecting unknown workflows.
- Pushes to `main` deploy successfully to
  `https://gjorgjidimeski.github.io/pulumi-dokploy/`.
