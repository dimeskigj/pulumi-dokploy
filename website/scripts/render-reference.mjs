import { access, mkdir, mkdtemp, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join, basename } from "node:path";

function frontmatter(title, description) {
  return `---\ntitle: ${title}\ndescription: ${JSON.stringify(description)}\n---\n`;
}

const generatedNotice = "> Generated from `schema.json`. Add lifecycle guidance to curated guides, not this file.";
const propertyTableImport = 'import PropertyTable from "../../../components/PropertyTable.astro";';

export function renderResource(resource) {
  const replacementMetadata = [...new Map([...resource.inputs, ...resource.outputs]
    .filter((property) => property.replaceOnChanges)
    .map((property) => [property.name, property])).values()]
    .map((property) => `<!-- ${property.name} replaceOnChanges: true -->`)
    .join("\n");
  return `${frontmatter(resource.name, resource.description)}
${propertyTableImport}

${generatedNotice}
${replacementMetadata ? `\n\n${replacementMetadata}` : ""}

${resource.description}

## Inputs

<PropertyTable properties={${JSON.stringify(resource.inputs)}} />

## Outputs

<PropertyTable properties={${JSON.stringify(resource.outputs)}} />
`;
}

export function renderConfiguration(model) {
  return `${frontmatter("Configuration", "Configure the Dokploy provider.")}
${propertyTableImport}

${generatedNotice}

Provider configuration for Dokploy.

## Configuration

<PropertyTable properties={${JSON.stringify(model.config)}} />
`;
}

export function renderTypes(model) {
  const types = [...model.types].sort((left, right) => left.name.localeCompare(right.name));
  const sections = types.map((type) => `## ${type.name} {#${type.slug}}\n\n${type.description}\n\n<PropertyTable properties={${JSON.stringify(type.properties)}} />`).join("\n\n");
  return `${frontmatter("Types", "Complex types used by the Dokploy provider.")}
${propertyTableImport}

${generatedNotice}

${sections}
`;
}

async function pathExists(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}

async function uniqueSiblingPath(target, label) {
  const path = await mkdtemp(join(dirname(target), `.${basename(target)}-${label}-`));
  await rm(path, { recursive: true, force: true });
  return path;
}

export async function replaceGeneratedDirectory(target, files, write = writeFile) {
  const temporary = await mkdtemp(join(dirname(target), `.${basename(target)}-tmp-`));
  let backup;
  let targetMoved = false;
  let installed = false;

  try {
    for (const [filename, content] of Object.entries(files)) {
      await write(join(temporary, filename), content, "utf8");
    }

    if (await pathExists(target)) {
      backup = await uniqueSiblingPath(target, "backup");
      await rename(target, backup);
      targetMoved = true;
    }

    try {
      await rename(temporary, target);
      installed = true;
    } catch (error) {
      if (targetMoved) {
        try {
          await rename(backup, target);
          backup = undefined;
          targetMoved = false;
        } catch {
          // Preserve the failed installation error; cleanup below is best effort.
        }
      }
      throw error;
    }

    if (backup) {
      await rm(backup, { recursive: true, force: true });
      backup = undefined;
    }
  } finally {
    try {
      if (!installed) await rm(temporary, { recursive: true, force: true });
    } catch {
      // Cleanup must not mask generation or installation errors.
    }
    try {
      if (backup) await rm(backup, { recursive: true, force: true });
    } catch {
      // Cleanup must not mask generation or installation errors.
    }
  }
}
