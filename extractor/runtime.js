#!/usr/bin/env node
/**
 * rute runtime extractor — uses esbuild to bundle a .ts file, evaluates it,
 * then uses z.toJSONSchema() to introspect the schema and outputs JSON.
 *
 * Usage: node extractor/runtime.js <file.ts> <ExportName>
 *
 * Requires: esbuild and zod in the project's node_modules.
 * Falls back to static extractor if dependencies are missing.
 */

'use strict';

const fs = require('fs');
const path = require('path');

const [, , filePath, exportName] = process.argv;

if (!filePath || !exportName) {
  process.stderr.write('Usage: node extractor/runtime.js <file.ts> <ExportName>\n');
  process.exit(1);
}

const absFile = path.resolve(filePath);
if (!fs.existsSync(absFile)) {
  process.stderr.write(`File not found: ${filePath}\n`);
  process.exit(1);
}

// Find the project root (nearest directory with node_modules)
function findProjectRoot(startDir) {
  let dir = startDir;
  while (dir !== path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, 'node_modules'))) return dir;
    dir = path.dirname(dir);
  }
  return null;
}

const projectRoot = findProjectRoot(path.dirname(absFile));
if (!projectRoot) {
  process.stderr.write('FALLBACK: no node_modules found\n');
  process.exit(2); // signal caller to fall back to static
}

// Try to load esbuild and zod
let esbuild, zod;
try {
  esbuild = require(path.join(projectRoot, 'node_modules', 'esbuild'));
} catch {
  process.stderr.write('FALLBACK: esbuild not found in project\n');
  process.exit(2);
}
try {
  zod = require(path.join(projectRoot, 'node_modules', 'zod'));
} catch {
  process.stderr.write('FALLBACK: zod not found in project\n');
  process.exit(2);
}

if (typeof zod.toJSONSchema !== 'function') {
  process.stderr.write('FALLBACK: z.toJSONSchema not available (Zod < 4)\n');
  process.exit(2);
}

// ---------------------------------------------------------------------------
// Bundle the target file with esbuild, eval to get live schema
// ---------------------------------------------------------------------------

async function run() {
  // Bundle a wrapper that re-exports the named schema
  const wrapper = `module.exports = require(${JSON.stringify(absFile)});`;

  const result = await esbuild.build({
    stdin: {
      contents: wrapper,
      resolveDir: path.dirname(absFile),
      loader: 'js',
    },
    bundle: true,
    write: false,
    format: 'cjs',
    platform: 'node',
    logLevel: 'silent',
    // Mark Node builtins as external
    packages: 'external',
  });

  if (result.errors.length > 0) {
    process.stderr.write(`esbuild errors: ${JSON.stringify(result.errors)}\n`);
    process.exit(1);
  }

  const bundledCode = result.outputFiles[0].text;

  // Evaluate the bundle
  const Module = require('module');
  const m = new Module('rute-bundle');
  m.paths = Module._nodeModulePaths(path.dirname(absFile));
  m._compile(bundledCode, 'rute-bundle.js');
  const mod = m.exports;

  const schema = mod[exportName];
  if (!schema) {
    process.stderr.write(`Export not found: ${exportName} in ${filePath}\n`);
    process.exit(1);
  }

  // Ensure it's a Zod schema
  if (!schema._def && !schema.type) {
    process.stderr.write(`Export ${exportName} is not a Zod schema\n`);
    process.exit(1);
  }

  // Convert to JSON Schema
  const jsonSchema = zod.toJSONSchema(schema);

  // Transform JSON Schema → rute format
  const output = jsonSchemaToRute(jsonSchema, exportName);
  process.stdout.write(JSON.stringify(output, null, 2));
}

// ---------------------------------------------------------------------------
// JSON Schema → rute format converter
// ---------------------------------------------------------------------------

function jsonSchemaToRute(js, name) {
  if (js.type === 'object') {
    const fields = [];
    const required = new Set(js.required || []);
    for (const [key, prop] of Object.entries(js.properties || {})) {
      fields.push(jsonSchemaFieldToRute(key, prop, required.has(key)));
    }
    return { name, type: 'object', fields };
  }

  if (js.type === 'array') {
    const items = js.items ? jsonSchemaToRute(js.items, '') : undefined;
    return { name, type: 'array', items };
  }

  if (js.enum) {
    return { name, type: 'enum', values: js.enum };
  }

  if (js.anyOf) {
    // Check for nullable pattern: anyOf[realType, {type: "null"}]
    const nonNull = js.anyOf.filter(v => v.type !== 'null');
    const hasNull = js.anyOf.some(v => v.type === 'null');
    if (hasNull && nonNull.length === 1) {
      const inner = jsonSchemaToRute(nonNull[0], name);
      return inner; // nullable is handled at field level
    }
    // General union
    const variants = js.anyOf.map(v => jsonSchemaToRute(v, ''));
    return { name, type: 'union', variants };
  }

  if (js.type) {
    return { name, type: js.type };
  }

  return { name, type: 'unknown' };
}

function jsonSchemaFieldToRute(fieldName, prop, isRequired) {
  const validations = [];
  let type = prop.type || 'unknown';
  let nullable = false;
  let values = undefined;
  let fields = undefined;
  let items = undefined;
  let defaultValue = undefined;
  let description = prop.description;

  // Handle anyOf (nullable or union)
  if (prop.anyOf) {
    const nonNull = prop.anyOf.filter(v => v.type !== 'null');
    const hasNull = prop.anyOf.some(v => v.type === 'null');
    if (hasNull) nullable = true;
    if (nonNull.length === 1) {
      // Nullable wrapper — unwrap and re-process
      const inner = { ...nonNull[0] };
      if (prop.default !== undefined) inner.default = prop.default;
      if (prop.description) inner.description = prop.description;
      const result = jsonSchemaFieldToRute(fieldName, inner, isRequired);
      result.nullable = true;
      return result;
    }
  }

  // Handle default
  if (prop.default !== undefined) {
    defaultValue = prop.default;
  }

  // Handle enum
  if (prop.enum) {
    type = 'enum';
    values = prop.enum;
  }

  // Handle object
  if (type === 'object' && prop.properties) {
    const required = new Set(prop.required || []);
    fields = [];
    for (const [key, subProp] of Object.entries(prop.properties)) {
      fields.push(jsonSchemaFieldToRute(key, subProp, required.has(key)));
    }
  }

  // Handle array
  if (type === 'array' && prop.items) {
    items = jsonSchemaToRute(prop.items, '');
  }

  // Collect validations from JSON Schema keywords
  if (prop.format) {
    validations.push(prop.format); // email, uuid, url, etc.
  }
  if (prop.minLength !== undefined) validations.push(`min:${prop.minLength}`);
  if (prop.maxLength !== undefined) validations.push(`max:${prop.maxLength}`);
  // Filter out JS safe integer bounds — Zod v4 emits these for every .int()
  const SAFE_MIN = -9007199254740991; // Number.MIN_SAFE_INTEGER
  const SAFE_MAX = 9007199254740991;  // Number.MAX_SAFE_INTEGER
  if (prop.minimum !== undefined && prop.minimum !== SAFE_MIN) validations.push(`min:${prop.minimum}`);
  if (prop.maximum !== undefined && prop.maximum !== SAFE_MAX) validations.push(`max:${prop.maximum}`);
  if (prop.exclusiveMinimum !== undefined && prop.exclusiveMinimum !== SAFE_MIN) validations.push(`exclusiveMin:${prop.exclusiveMinimum}`);
  if (prop.exclusiveMaximum !== undefined && prop.exclusiveMaximum !== SAFE_MAX) validations.push(`exclusiveMax:${prop.exclusiveMaximum}`);
  if (prop.multipleOf !== undefined) {
    if (prop.multipleOf === 1) validations.push('int');
    else validations.push(`multipleOf:${prop.multipleOf}`);
  }
  if (prop.pattern && !prop.format) {
    // Only show pattern if there's no format (format already implies a pattern)
    validations.push(`regex:${prop.pattern}`);
  }

  const field = {
    name: fieldName,
    type,
    required: isRequired,
  };

  if (nullable) field.nullable = true;
  if (defaultValue !== undefined) field.default = String(defaultValue);
  if (description) field.description = description;
  if (validations.length > 0) field.validations = validations;
  if (values) field.values = values;
  if (fields) field.fields = fields;
  if (items) field.items = items;

  return field;
}

run().catch(err => {
  process.stderr.write(`Runtime extractor error: ${err.message}\n`);
  process.exit(1);
});
