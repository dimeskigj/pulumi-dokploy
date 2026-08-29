import assert from "node:assert/strict";
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

test("loads and validates the real provider schema", async () => {
  const model = parseSchema(
    await loadSchema(new URL("../../provider/cmd/pulumi-resource-dokploy/schema.json", import.meta.url)),
  );
  assert.equal(model.resources.length, 7);
  assert.equal(model.config.find(({ name }) => name === "apiKey").secret, true);
  assert.equal(
    model.resources
      .find(({ name }) => name === "Application")
      .inputs.find(({ name }) => name === "environmentId").replaceOnChanges,
    true,
  );
});
