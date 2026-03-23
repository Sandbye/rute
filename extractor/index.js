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
// Import resolution — parse import statements and follow cross-file refs
// ---------------------------------------------------------------------------

/** Cache of already-read file sources (absolute path → source string). */
const fileCache = new Map();
fileCache.set(path.resolve(filePath), source);

/** Parse import statements from source, return map of name → { file, name }. */
function parseImports(src, fromFile) {
  const imports = new Map();
  const re = /import\s+\{([^}]+)\}\s+from\s+['"]([^'"]+)['"]/g;
  let m;
  while ((m = re.exec(src)) !== null) {
    const names = m[1].split(',').map(s => s.trim()).filter(Boolean);
    let importPath = m[2];
    // Resolve relative to the importing file's directory
    if (importPath.startsWith('.')) {
      importPath = path.resolve(path.dirname(fromFile), importPath);
      // Try .ts extension if not present
      if (!importPath.endsWith('.ts') && !fs.existsSync(importPath)) {
        importPath += '.ts';
      }
    }
    for (const n of names) {
      // Handle `Name as Alias`
      const parts = n.split(/\s+as\s+/);
      const originalName = parts[0].trim();
      const localName = (parts[1] || parts[0]).trim();
      imports.set(localName, { file: importPath, name: originalName });
    }
  }
  return imports;
}

/** Read a file, cached. */
function readFile(absPath) {
  if (fileCache.has(absPath)) return fileCache.get(absPath);
  try {
    const src = fs.readFileSync(absPath, 'utf8');
    fileCache.set(absPath, src);
    return src;
  } catch {
    return null;
  }
}

/**
 * Resolve a bare identifier (schema name) to its Zod expression body.
 * Searches current file first, then follows imports.
 * Returns { expr, src, file } or null.
 */
function resolveIdentifier(name, currentSrc, currentFile, visited) {
  visited = visited || new Set();
  const key = `${currentFile}:${name}`;
  if (visited.has(key)) return null;
  visited.add(key);

  // Try finding in current file
  const body = findExportBody(currentSrc, name);
  if (body) return { expr: body, src: currentSrc, file: currentFile };

  // Also check non-exported const (local variables)
  const localRe = new RegExp(`(?:const|let|var)\\s+${name}\\s*=\\s*`, 'm');
  const localMatch = localRe.exec(currentSrc);
  if (localMatch) {
    const start = localMatch.index + localMatch[0].length;
    const localBody = extractBalancedExpression(currentSrc, start);
    if (localBody) return { expr: localBody, src: currentSrc, file: currentFile };
  }

  // Check imports
  const imports = parseImports(currentSrc, currentFile);
  const imp = imports.get(name);
  if (imp) {
    const absFile = path.resolve(imp.file);
    const importedSrc = readFile(absFile);
    if (importedSrc) {
      return resolveIdentifier(imp.name, importedSrc, absFile, visited);
    }
  }

  return null;
}

/**
 * Parse a schema expression, resolving identifier references and
 * applying object transforms (.extend, .merge, .pick, .omit, .partial).
 */
function parseZodExprInContext(expr, name, src, file) {
  expr = expr.trim().replace(/;+$/, '').trim();

  // Strip object transforms from the end, collecting them in order.
  // We must always strip the LAST chained call first to avoid swallowing
  // later transforms inside earlier ones.
  const transforms = [];
  let base = expr;

  let changed = true;
  while (changed) {
    changed = false;

    // Find the last top-level .method( in the chain by scanning from the end.
    const lastTransform = findLastTransform(base);
    if (!lastTransform) break;

    const { type, idx } = lastTransform;
    const tail = base.slice(idx);
    const inner = extractArgs(tail, '');

    switch (type) {
      case 'partial':
        transforms.unshift({ type: 'partial' });
        break;
      case 'extend':
        transforms.unshift({ type: 'extend', fields: inner });
        break;
      case 'merge':
        transforms.unshift({ type: 'merge', ref: inner.trim() });
        break;
      case 'pick':
        transforms.unshift({ type: 'pick', keys: parseKeySet(inner) });
        break;
      case 'omit':
        transforms.unshift({ type: 'omit', keys: parseKeySet(inner) });
        break;
    }

    base = base.slice(0, idx);
    changed = true;
  }

  // Parse the base expression
  let schema;

  // Check if base is a bare identifier (e.g., `BaseSchema`)
  if (/^[A-Za-z_$][\w$]*$/.test(base) && !base.startsWith('z.')) {
    const resolved = resolveIdentifier(base, src, file);
    if (resolved) {
      schema = parseZodExprInContext(resolved.expr, name, resolved.src, resolved.file);
    } else {
      schema = { name, type: 'unknown' };
    }
  } else {
    schema = parseZodExpr(base, name);
  }

  // Apply transforms in order
  for (const t of transforms) {
    if (schema.type !== 'object') continue;
    switch (t.type) {
      case 'extend': {
        const newFields = parseObjectFields(t.fields);
        // New fields override existing ones with same name
        const existing = new Map(schema.fields.map(f => [f.name, f]));
        for (const f of newFields) existing.set(f.name, f);
        schema.fields = [...existing.values()];
        break;
      }
      case 'merge': {
        const resolved = resolveIdentifier(t.ref, src, file);
        if (resolved) {
          const other = parseZodExprInContext(resolved.expr, '', resolved.src, resolved.file);
          if (other.fields) {
            const existing = new Map(schema.fields.map(f => [f.name, f]));
            for (const f of other.fields) existing.set(f.name, f);
            schema.fields = [...existing.values()];
          }
        }
        break;
      }
      case 'pick': {
        schema.fields = schema.fields.filter(f => t.keys.has(f.name));
        break;
      }
      case 'omit': {
        schema.fields = schema.fields.filter(f => !t.keys.has(f.name));
        break;
      }
      case 'partial': {
        schema.fields = schema.fields.map(f => ({ ...f, required: false }));
        break;
      }
    }
  }

  return schema;
}

/**
 * Find the last top-level (depth 0) transform call in an expression.
 * Returns { type, idx } or null.
 */
function findLastTransform(expr) {
  const transformNames = ['extend', 'merge', 'pick', 'omit', 'partial'];
  let depth = 0;
  let best = null; // { type, idx } with highest idx

  for (let i = 0; i < expr.length; i++) {
    const ch = expr[i];

    // Skip strings
    if (ch === '"' || ch === "'" || ch === '`') {
      const q = ch;
      i++;
      while (i < expr.length) {
        if (expr[i] === '\\') { i += 2; continue; }
        if (expr[i] === q) break;
        i++;
      }
      continue;
    }

    if (ch === '(' || ch === '[' || ch === '{') { depth++; continue; }
    if (ch === ')' || ch === ']' || ch === '}') { depth--; continue; }

    // At depth 0, check for .transformName(
    if (depth === 0 && ch === '.') {
      for (const name of transformNames) {
        if (expr.slice(i + 1, i + 1 + name.length) === name) {
          // Check next char after name is ( or whitespace then (
          let j = i + 1 + name.length;
          while (j < expr.length && /\s/.test(expr[j])) j++;
          if (expr[j] === '(') {
            best = { type: name, idx: i };
          }
        }
      }
    }
  }

  return best;
}

/** Parse `{ key: true, ... }` into a Set of key names. */
function parseKeySet(inner) {
  inner = inner.trim();
  if (inner.startsWith('{')) inner = inner.slice(1);
  if (inner.endsWith('}')) inner = inner.slice(0, -1);
  const keys = new Set();
  for (const part of inner.split(',')) {
    const key = part.split(':')[0].trim().replace(/['"]/g, '');
    if (key) keys.add(key);
  }
  return keys;
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
    defaultValue = parseDefaultValue(defaultMatch[1].trim());
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
  if (schema.variants) field.variants = schema.variants;

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

function parseDefaultValue(raw) {
  const value = raw.trim();
  if (/^['"`].*['"`]$/.test(value)) {
    return value.slice(1, -1);
  }
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (value === 'null') return null;
  if (/^-?\d+(?:\.\d+)?$/.test(value)) {
    return Number(value);
  }
  return value;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

const body = findExportBody(source, exportName);
if (!body) {
  process.stderr.write(`Export not found: ${exportName} in ${filePath}\n`);
  process.exit(1);
}

const schema = parseZodExprInContext(body, exportName, source, path.resolve(filePath));
process.stdout.write(JSON.stringify(schema, null, 2));
