import { readFile } from "node:fs/promises";

const EXPECTED_RESOURCES = new Set([
  "Application", "Compose", "Domain", "Environment", "Postgres", "MySQL", "MariaDB", "MongoDB", "Project", "Redis",
  "Destination", "Backup", "VolumeBackup", "SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "Schedule", "Notification",
]);
const SOURCE_PATH = Symbol("schema source path");

export async function loadSchema(path) {
  let source;
  try {
    source = await readFile(path, "utf8");
    const schema = JSON.parse(source);
    if (schema && typeof schema === "object") {
      Object.defineProperty(schema, SOURCE_PATH, { value: String(path) });
    }
    return schema;
  } catch (error) {
    throw new Error(`Unable to load schema ${path}: ${error.message}`, { cause: error });
  }
}

// Names whose branded spelling (mysql, mariadb, mongodb) doesn't match what
// the generic camelCase-to-kebab split below would produce (my-sql,
// maria-db, mongo-db) because they pack multiple capitalized segments,
// including an acronym, together.
const SLUG_OVERRIDES = { MySQL: "mysql", MariaDB: "mariadb", MongoDB: "mongodb" };

export function slugFromToken(token) {
  if (typeof token !== "string" || !/^dokploy:index:[A-Za-z][A-Za-z0-9_-]*$/.test(token)) {
    throw new Error(`Invalid Pulumi token: ${token}`);
  }
  const name = token.split(":").at(-1);
  if (!name) throw new Error(`Invalid Pulumi token: ${token}`);
  if (SLUG_OVERRIDES[name]) return SLUG_OVERRIDES[name];
  return name.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

export function formatType(property) {
  if (property.$ref) return property.$ref.split(":").at(-1);
  if (property.type === "array" && property.items) return `${formatType(property.items)}[]`;
  if (property.type === "object" && property.additionalProperties?.type === "string" &&
      Object.keys(property.additionalProperties).length === 1 &&
      Object.keys(property).every((key) => ["type", "additionalProperties", "description", "secret"].includes(key))) {
    return "map<string, string>";
  }
  if (["string", "integer", "number", "boolean"].includes(property.type)) return property.type;
  throw new Error(`Unsupported Pulumi property type: ${JSON.stringify(property)}`);
}

function resourceName(token) {
  slugFromToken(token);
  return token.split(":").at(-1);
}

function assertDescription(description, context) {
  if (typeof description !== "string" || !description.trim()) {
    throw new Error(`Missing description for ${context}`);
  }
  return description;
}

function referenceToken(property) {
  if (!property.$ref) return null;
  const prefix = "#/types/";
  if (!property.$ref.startsWith(prefix)) {
    throw new Error(`Invalid type reference: ${property.$ref}`);
  }
  return property.$ref.slice(prefix.length);
}

function normalizeProperties(properties = {}, requiredNames = [], context, types) {
  const required = new Set(requiredNames);
  return Object.keys(properties).sort().map((name) => {
    const property = properties[name];
    const reference = referenceToken(property);
    if (reference && !Object.hasOwn(types, reference)) {
      throw new Error(`Dangling type reference ${property.$ref} in ${context}.${name}`);
    }
    const defaultValue = ["string", "number", "boolean"].includes(typeof property.default)
      ? property.default
      : null;
    return {
      name,
      type: formatType(property),
      typeHref: reference ? `../types/#${slugFromToken(reference)}` : null,
      description: assertDescription(property.description, `${context}.${name}`),
      required: required.has(name),
      secret: property.secret === true,
      replaceOnChanges: property.replaceOnChanges === true,
      defaultValue,
      environment: property.defaultInfo?.environment ?? [],
    };
  });
}

function parseSchemaInternal(schema, options = {}) {
  if (!schema || schema.name !== "dokploy") {
    throw new Error(`Unexpected Pulumi package name: ${schema?.name}`);
  }
  const types = schema.types ?? {};
  const resources = schema.resources ?? {};
  const expectedResources = options.expectedResources ?? EXPECTED_RESOURCES;
  const resourceTokens = Object.keys(resources);
  const expectedTokens = new Set([...expectedResources].map((name) => `dokploy:index:${name}`));
  if (
    resourceTokens.length !== expectedTokens.size ||
    resourceTokens.some((token) => !expectedTokens.has(token))
  ) {
    throw new Error(`Unexpected resource set: ${resourceTokens.join(", ")}`);
  }

  const typeModels = Object.keys(types).sort().map((token) => {
    const definition = types[token];
    const name = resourceName(token);
    // Nested Notification types have annotated properties but Pulumi inference
    // publishes no type-level description for them. Keep other types strict.
    const notificationDescription = /^Notification(?:Events|(?:Slack|Telegram|Discord|Email|Resend|Gotify|Ntfy|Mattermost|Custom|Lark|Teams|Pushover)Config)$/.test(name)
      ? `${name === "NotificationEvents" ? "Event subscriptions" : `${name.slice(12, -6)} channel settings`} for a Dokploy notification.`
      : null;
    return {
      token,
      name,
      slug: slugFromToken(token),
      description: assertDescription(definition.description || notificationDescription, `type ${token}`),
      properties: normalizeProperties(definition.properties, definition.required, `type ${token}`, types),
    };
  });
  const resourceModels = Object.keys(resources).sort().map((token) => {
    const definition = resources[token];
    const name = resourceName(token);
    return {
      token,
      name,
      slug: slugFromToken(token),
      description: assertDescription(definition.description, `resource ${token}`),
      inputs: normalizeProperties(definition.inputProperties, definition.requiredInputs, `resource ${token} inputs`, types),
      outputs: normalizeProperties(definition.properties, definition.required, `resource ${token} outputs`, types),
    };
  });
  return {
    config: normalizeProperties(schema.config?.variables, schema.config?.required, "config", types),
    resources: resourceModels,
    types: typeModels,
  };
}

export function parseSchema(schema, options = {}) {
  try {
    return parseSchemaInternal(schema, options);
  } catch (error) {
    const source = schema?.[SOURCE_PATH];
    if (source && !error.message.includes(source)) {
      throw new Error(`${error.message} (schema: ${source})`, { cause: error });
    }
    throw error;
  }
}
