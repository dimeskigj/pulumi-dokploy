import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { loadSchema, parseSchema } from "./reference-model.mjs";
import { renderConfiguration, renderResource, renderTypes, replaceGeneratedDirectory } from "./render-reference.mjs";

try {
  const scriptsDirectory = fileURLToPath(new URL(".", import.meta.url));
  const schemaPath = join(scriptsDirectory, "../../provider/cmd/pulumi-resource-dokploy/schema.json");
  const target = join(scriptsDirectory, "../src/content/docs/reference");
  const model = parseSchema(await loadSchema(schemaPath));
  const files = {
    "configuration.mdx": renderConfiguration(model),
    "types.mdx": renderTypes(model),
  };
  for (const resource of [...model.resources].sort((left, right) => left.name.localeCompare(right.name))) {
    files[`${resource.slug}.mdx`] = renderResource(resource);
  }
  await replaceGeneratedDirectory(target, files);
} catch (error) {
  console.error("Reference generation failed: " + error.message);
  process.exitCode = 1;
}
