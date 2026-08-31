import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { formatType, loadSchema, parseSchema, slugFromToken } from "../scripts/reference-model.mjs";

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

test("rejects tokens that are not exactly dokploy:index:identifier", () => {
  for (const token of [
    "dokploy:other:Postgres",
    "dokploy:index:Postgres:Extra",
    "dokploy:index:",
    "other:index:Postgres",
    "dokploy:index:Postgres/unsafe",
  ]) {
    assert.throws(() => slugFromToken(token), /Invalid Pulumi token/);
  }
});

test("loads and validates the real provider schema", async () => {
  const model = parseSchema(
    await loadSchema(new URL("../../provider/cmd/pulumi-resource-dokploy/schema.json", import.meta.url)),
  );
  assert.equal(model.resources.length, 13);
  assert.equal(model.config.find(({ name }) => name === "apiKey").secret, true);
  assert.equal(
    model.resources
      .find(({ name }) => name === "Application")
      .inputs.find(({ name }) => name === "environmentId").replaceOnChanges,
    true,
  );
});

test("rejects duplicate normalized resource names", () => {
  const resources = {
    ...schema.resources,
    "dokploy:index:Nested:Application": schema.resources["dokploy:index:Application"],
  };
  assert.throws(
    () => parseSchema({ ...schema, resources }, { expectedResources: new Set(["Application"]) }),
    /Unexpected resource set/,
  );
});

test("rejects resource tokens that do not exactly match expected tokens", () => {
  const resources = { "dokploy:other:Application": schema.resources["dokploy:index:Application"] };
  assert.throws(
    () => parseSchema({ ...schema, resources }, { expectedResources: new Set(["Application"]) }),
    /Unexpected resource set/,
  );
});

test("rejects missing descriptions and dangling type references", () => {
  const missingDescription = structuredClone(schema);
  missingDescription.resources["dokploy:index:Application"].description = "";
  assert.throws(
    () => parseSchema(missingDescription, { expectedResources: new Set(["Application"]) }),
    /Missing description/,
  );

  const danglingReference = structuredClone(schema);
  danglingReference.resources["dokploy:index:Application"].inputProperties.source.$ref =
    "#/types/dokploy:index:Missing";
  assert.throws(
    () => parseSchema(danglingReference, { expectedResources: new Set(["Application"]) }),
    /Dangling type reference/,
  );
});

test("includes the source path in validation errors for loaded schemas", async () => {
  const directory = await mkdtemp(join(tmpdir(), "reference-model-"));
  const path = join(directory, "malformed-schema.json");
  try {
    await writeFile(path, JSON.stringify({ ...schema, name: "not-dokploy" }));
    const loaded = await loadSchema(path);
    assert.throws(() => parseSchema(loaded), new RegExp(`not-dokploy.*${path}`));
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
