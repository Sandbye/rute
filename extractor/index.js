#!/usr/bin/env node
/**
 * rute extractor — parses a TypeScript file statically and extracts
 * the shape of a named Zod schema export as JSON.
 *
 * Usage: node extractor/index.js <file.ts> <ExportName>
 *
 * Outputs JSON to stdout. Exits non-zero on error (message to stderr).
 */

'use strict';

const fs = require('fs');
const path = require('path');

const [, , filePath, exportName] = process.argv;

if (!filePath || !exportName) {
  process.stderr.write('Usage: node extractor/index.js <file.ts> <ExportName>\n');
  process.exit(1);
}

let source;
try {
  source = fs.readFileSync(filePath, 'utf8');
} catch (err) {
  process.stderr.write(`Could not read file: ${filePath}\n`);
  process.exit(1);
}

// ---------------------------------------------------------------------------
// Minimal static parser for Zod schemas.
// Strategy: find the export assignment, then walk the Zod expression tree
// using regex + balanced-bracket scanning. No AST — keeps the dependency
// list at zero.
// ---------------------------------------------------------------------------

/**
 * Find the source text of an exported const assignment.
 * Handles both:
 *   export const Foo = ...
 *   const Foo = ...; export { Foo };
 */
function findExportBody(src, name) {
  // Match: export const Name = <body>
  const inlineRe = new RegExp(
    `(?:export\\s+const|const)\\s+${name}\\s*=\\s*`,
    'm'
  );
  const match = inlineRe.exec(src);
  if (!match) return null;

  const start = match.index + match[0].length;
  return extractBalancedExpression(src, start);
}

/**
 * Extract a full Zod expression starting at `start`, including chained
 * method calls like `.optional()`, `.uuid()`, `.min(8)`, etc.
 * Stops at `,` or `}` at depth 0.
 */
function extractBalancedExpression(src, start) {
  let i = start;

  // Skip leading whitespace
  while (i < src.length && /\s/.test(src[i])) i++;

  if (i >= src.length) return '';

  // Consume the full expression: identifier segments, dots, and balanced parens/brackets/braces
  let depth = 0;
  let exprStart = i;

  while (i < src.length) {
    const ch = src[i];

    // Skip string literals
    if (ch === '"' || ch === "'" || ch === '`') {
      const quote = ch;
      i++;
      while (i < src.length) {
        if (src[i] === '\\') { i += 2; continue; }
        if (src[i] === quote) { i++; break; }
        i++;
      }
      continue;
    }

    if (ch === '(' || ch === '[' || ch === '{') {
      depth++;
    } else if (ch === ')' || ch === ']' || ch === '}') {
      if (depth === 0) {
        // We've hit the closing delimiter of the parent scope — stop.
        break;
      }
      depth--;
    } else if (depth === 0 && ch === ',') {
      // Field separator — stop.
      break;
    } else if (depth === 0 && ch === '\n') {
      // Check if next non-whitespace continues a chain (dot)
      let j = i + 1;
      while (j < src.length && /[ \t]/.test(src[j])) j++;
      if (src[j] !== '.') break;
      // continuation — keep going (skip the newline)
    }

    i++;
  }

  return src.slice(exprStart, i).trim();
}

/**
 * Parse a Zod expression string into a Schema object.
 */
function parseZodExpr(expr, name) {
  expr = expr.trim();

  // z.object({...})
  if (/^z\.object\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.object');
    const fields = parseObjectFields(inner);
    return { name, type: 'object', fields };
  }

  // z.array(...)
  if (/^z\.array\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.array');
    const items = parseZodExpr(inner, '');
    return { name, type: 'array', items };
  }

  // z.enum([...])
  if (/^z\.enum\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.enum');
    const values = parseStringArray(inner);
    return { name, type: 'enum', values };
  }

  // z.union([...])
  if (/^z\.union\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.union');
    const variants = parseUnionVariants(inner);
    return { name, type: 'union', variants };
  }

  // z.discriminatedUnion(key, [...])
  if (/^z\.discriminatedUnion\s*\(/.test(expr)) {
    return { name, type: 'union', variants: [] };
  }

  // z.intersection(A, B)
  if (/^z\.intersection\s*\(/.test(expr)) {
    return { name, type: 'intersection' };
  }

  // z.record(...)
  if (/^z\.record\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.record');
    const value = parseZodExpr(inner, '');
    return { name, type: 'record', items: value };
  }

  // z.string()
  if (/^z\.string\s*\(/.test(expr)) {
    return { name, type: 'string' };
  }

  // z.number()
  if (/^z\.number\s*\(/.test(expr)) {
    return { name, type: 'number' };
  }

  // z.boolean()
  if (/^z\.boolean\s*\(/.test(expr)) {
    return { name, type: 'boolean' };
  }

  // z.date()
  if (/^z\.date\s*\(/.test(expr)) {
    return { name, type: 'date' };
  }

  // z.any()
  if (/^z\.any\s*\(/.test(expr)) {
    return { name, type: 'any' };
  }

  // z.unknown()
  if (/^z\.unknown\s*\(/.test(expr)) {
    return { name, type: 'unknown' };
  }

  // z.null()
  if (/^z\.null\s*\(/.test(expr)) {
    return { name, type: 'null' };
  }

  // z.literal(...)
  if (/^z\.literal\s*\(/.test(expr)) {
    const inner = extractArgs(expr, 'z.literal').trim();
    return { name, type: 'literal', values: [inner.replace(/['"]/g, '')] };
  }

  return { name, type: 'unknown' };
}

/**
 * Extract the first argument list from a call like `z.object({...}).optional()`.
 * Returns the text inside the first balanced parens.
 */
function extractArgs(expr, prefix) {
  const start = expr.indexOf('(');
  if (start === -1) return '';
  // The expression may have chained calls after the closing paren.
  // Find the matching close paren for the first open paren.
  let depth = 0;
  let i = start;
  while (i < expr.length) {
    if (expr[i] === '(') depth++;
    else if (expr[i] === ')') {
      depth--;
      if (depth === 0) return expr.slice(start + 1, i);
    }
    i++;
  }
  return expr.slice(start + 1);
}

/**
 * Parse `{ key: z.string(), ... }` into an array of Field objects.
 */
function parseObjectFields(inner) {
  inner = inner.trim();
  // Remove surrounding braces if present
  if (inner.startsWith('{') && inner.endsWith('}')) {
    inner = inner.slice(1, -1).trim();
  }

  const fields = [];
  let i = 0;

  while (i < inner.length) {
    // Skip whitespace
    while (i < inner.length && /\s/.test(inner[i])) i++;
    if (i >= inner.length) break;

    // Read field name (identifier or quoted string)
    let fieldName = '';
    if (inner[i] === '"' || inner[i] === "'") {
      const q = inner[i++];
      while (i < inner.length && inner[i] !== q) fieldName += inner[i++];
      i++; // closing quote
    } else {
      while (i < inner.length && /[\w$]/.test(inner[i])) fieldName += inner[i++];
    }

    if (!fieldName) { i++; continue; }

    // Skip whitespace + colon
    while (i < inner.length && /[\s:]/.test(inner[i])) i++;

    // Read the value expression (balanced)
    const valueStart = i;
    const valueExpr = extractBalancedExpression(inner, valueStart);
    i = valueStart + valueExpr.length;

    // Skip trailing comma
    while (i < inner.length && /[\s,]/.test(inner[i])) i++;

    const field = parseFieldExpr(fieldName, valueExpr.trim());
    fields.push(field);
  }

  return fields;
}

/**
 * Parse a single field expression into a Field object.
 * Handles chained modifiers: .optional(), .nullable(), .default(), .describe()
 */
function parseFieldExpr(name, expr) {
  let required = true;
  let nullable = false;
  let defaultValue = undefined;
  let description = undefined;
  const validations = [];

  // Strip chained modifiers and collect them
  let base = expr;

  // .describe("...")
  const describeMatch = /\.describe\s*\(\s*["'`]([^"'`]*)["'`]\s*\)/.exec(base);
  if (describeMatch) {
    description = describeMatch[1];
    base = base.replace(describeMatch[0], '');
  }

  // .default(...)
  const defaultMatch = /\.default\s*\(([^)]*)\)/.exec(base);
  if (defaultMatch) {
    defaultValue = defaultMatch[1].trim().replace(/['"]/g, '');
    base = base.replace(defaultMatch[0], '');
  }

  // .optional()
  if (/\.optional\s*\(\s*\)/.test(base)) {
    required = false;
    base = base.replace(/\.optional\s*\(\s*\)/, '');
  }

  // z.optional(...)
  if (/^z\.optional\s*\(/.test(base)) {
    required = false;
    base = extractArgs(base, 'z.optional');
  }

  // .nullable()
  if (/\.nullable\s*\(\s*\)/.test(base)) {
    nullable = true;
    base = base.replace(/\.nullable\s*\(\s*\)/, '');
  }

  // Collect string validators
  const stringValidators = ['email', 'uuid', 'url', 'cuid', 'min', 'max', 'length', 'regex', 'startsWith', 'endsWith'];
  for (const v of stringValidators) {
    const re = new RegExp(`\\.${v}\\s*\\(([^)]*)\\)`, 'g');
    let m;
    while ((m = re.exec(base)) !== null) {
      const arg = m[1].trim().replace(/['"]/g, '');
      validations.push(arg ? `${v}:${arg}` : v);
    }
    base = base.replace(new RegExp(`\\.${v}\\s*\\([^)]*\\)`, 'g'), '');
  }

  // Collect number validators
  const numberValidators = ['int', 'positive', 'negative', 'nonnegative', 'nonpositive', 'multipleOf'];
  for (const v of numberValidators) {
    const re = new RegExp(`\\.${v}\\s*\\(([^)]*)\\)`, 'g');
    let m;
    while ((m = re.exec(base)) !== null) {
      const arg = m[1].trim();
      validations.push(arg ? `${v}:${arg}` : v);
    }
    base = base.replace(new RegExp(`\\.${v}\\s*\\([^)]*\\)`, 'g'), '');
  }

  base = base.trim();

  // For object types, do not collect validations from nested field content.
  // Strip them only if this is not an object/array (they have their own parsers).
  const isContainer = /^z\.(object|array|union|record|discriminatedUnion|intersection)\s*\(/.test(base);
  if (isContainer) {
    validations.length = 0;
  }

  // Parse the base Zod type
  const schema = parseZodExpr(base, name);

  const field = {
    name,
    type: schema.type,
    required,
  };

  if (nullable) field.nullable = true;
  if (defaultValue !== undefined) field.default = defaultValue;
  if (description) field.description = description;
  if (validations.length > 0) field.validations = validations;
  if (schema.values) field.values = schema.values;
  if (schema.fields) field.fields = schema.fields;
  if (schema.items) field.items = schema.items;

  return field;
}

/**
 * Parse `["a", "b", "c"]` into `["a", "b", "c"]`.
 */
function parseStringArray(inner) {
  inner = inner.trim();
  if (inner.startsWith('[')) inner = inner.slice(1);
  if (inner.endsWith(']')) inner = inner.slice(0, -1);
  return inner
    .split(',')
    .map(s => s.trim().replace(/^['"`]|['"`]$/g, ''))
    .filter(Boolean);
}

/**
 * Parse union variants from `[z.object({...}), z.string(), ...]`.
 */
function parseUnionVariants(inner) {
  inner = inner.trim();
  if (inner.startsWith('[')) inner = inner.slice(1);
  if (inner.endsWith(']')) inner = inner.slice(0, -1);

  const variants = [];
  let depth = 0;
  let start = 0;

  for (let i = 0; i <= inner.length; i++) {
    const ch = inner[i];
    if (ch === '(' || ch === '[' || ch === '{') depth++;
    else if (ch === ')' || ch === ']' || ch === '}') depth--;
    else if ((ch === ',' || i === inner.length) && depth === 0) {
      const segment = inner.slice(start, i).trim();
      if (segment) variants.push(parseZodExpr(segment, ''));
      start = i + 1;
    }
  }

  return variants;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

const body = findExportBody(source, exportName);
if (!body) {
  process.stderr.write(`Export not found: ${exportName} in ${filePath}\n`);
  process.exit(1);
}

const schema = parseZodExpr(body, exportName);
process.stdout.write(JSON.stringify(schema, null, 2));
