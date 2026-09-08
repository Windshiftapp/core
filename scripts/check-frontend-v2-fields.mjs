#!/usr/bin/env node
/**
 * Guard: every statically-visible field the frontend serializes into a v2
 * write request must exist in the matching v2 OpenAPI request schema.
 *
 * Catches the WI-1283 regression class: a strict typed v2 handler rejecting
 * payload fields that were harmless under the legacy lenient decoders (DTO
 * round-trips, server-managed columns, forgotten create fields).
 *
 * Scope: fetchV2Data / fetchAPIV2 writes with a static (or template-literal)
 * URL and a literal `body: JSON.stringify({...})` options object. Computed
 * keys, spread payloads, and variable-built bodies are skipped; those need a
 * browser-surface test.
 *
 * Usage: bun run scripts/check-frontend-v2-fields.mjs
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = new URL('..', import.meta.url).pathname;
const specPath = join(root, 'api', 'openapi-v2.json');
const frontendDir = join(root, 'frontend', 'src');

const spec = JSON.parse(readFileSync(specPath, 'utf8'));

/** Pre-built index: normalized segment pattern -> [{path, method, operation}] */
const pathIndex = new Map();
for (const [rawPath, methods] of Object.entries(spec.paths)) {
  const pattern = pathPattern(rawPath);
  for (const [method, op] of Object.entries(methods)) {
    if (!['post', 'put', 'patch'].includes(method)) continue;
    const list = pathIndex.get(pattern) ?? [];
    list.push({ path: rawPath, method, operation: op });
    pathIndex.set(pattern, list);
  }
}

/** '/items/{item_id}/diagrams' -> 'items|#|diagrams' */
function pathPattern(path) {
  const clean = path.split('?')[0];
  return clean
    .split('/')
    .filter(Boolean)
    .map((segment) => (/^\{[^}]+\}$/.test(segment) ? '#' : segment))
    .join('|');
}

/** '/items/${itemId}/diagrams' -> segments; placeholders -> '#' */
function frontendPattern(template) {
  const clean = template.split('?')[0];
  return clean
    .split('/')
    .filter(Boolean)
    .map((segment) => (/\$\{?[A-Za-z_]\w*\}?/.test(segment) || segment.includes('\u0000') ? '#' : segment))
    .join('|');
}

function requestSchemaFor(operation) {
  const schema = operation?.requestBody?.content?.['application/json']?.schema;
  if (schema?.$ref) {
    const name = schema.$ref.split('/').pop();
    return spec.components?.schemas?.[name];
  }
  return null;
}

const errors = [];
let checked = 0;
let skippedPayloads = 0;

function walkFiles(dir) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (entry === 'node_modules' || entry === 'design-system') continue;
      walkFiles(full);
    } else if (full.endsWith('.js') || full.endsWith('.svelte')) {
      scanFile(full);
    }
  }
}

/** Return the extent of the balanced object starting at `start` (points at '{'). */
function objectExtent(source, start) {
  let depth = 0;
  let quote = null;
  for (let i = start; i < source.length; i++) {
    const ch = source[i];
    if (quote) {
      if (ch === '\\') { i += 1; continue; }
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === '`') { quote = ch; continue; }
    if (ch === '{') depth += 1;
    else if (ch === '}') {
      depth -= 1;
      if (depth === 0) return { start, end: i + 1 };
    }
  }
  return null;
}

/** Extract top-level keys and whether the object uses spread syntax. */
function objectShape(inner) {
  const keys = [];
  let spread = false;
  const pattern = /(?:^|[,{]\s*)(\.\.\.|'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"|([A-Za-z_$][\w$]*))\s*:/g;
  let match;
  while ((match = pattern.exec(inner)) !== null) {
    if (match[1] === '...') { spread = true; continue; }
    keys.push(match[2] ?? match[3] ?? match[4]);
  }
  return { keys, spread };
}

/** Read a backtick template starting at `i`; returns { text, next } tracking ${} nesting. */
function readTemplate(source, i) {
  let out = '';
  let depth = 0;
  for (; i < source.length; i++) {
    const ch = source[i];
    if (ch === '\\') { out += ch + (source[i + 1] ?? ''); i += 1; continue; }
    if (ch === '$' && source[i + 1] === '{') { depth += 1; out += '\u0000'; i += 1; continue; }
    if (ch === '}' && depth > 0) { depth -= 1; out += '\u0000'; continue; }
    if (ch === '`' && depth === 0) return { text: out, next: i + 1 };
    out += ch;
  }
  return { text: '', next: i };
}

function scanFile(file) {
  const source = readFileSync(file, 'utf8');
  const callRe = /fetch(V2Data|APIV2)\(/g;
  let call;
  while ((call = callRe.exec(source)) !== null) {
    const afterFn = call.index + call[0].length;
    const urlMatch = /^\s*(?:"([^"]+)"|'([^']+)')/.exec(source.slice(afterFn));
    let pathTemplate;
    let urlEnd;
    if (urlMatch) {
      pathTemplate = urlMatch[1] ?? urlMatch[2];
      urlEnd = afterFn + urlMatch[0].length;
    } else if (source[afterFn] === '`') {
      const template = readTemplate(source, afterFn + 1); // skip the opening backtick
      pathTemplate = template.text;
      urlEnd = template.next;
    }
    if (!pathTemplate) continue;
    const pattern = frontendPattern(pathTemplate);
    const candidates = pathIndex.get(pattern) ?? [];
    if (candidates.length === 0) continue; // GET/unknown route: nothing to check

    const optionsStart = source.indexOf(',', urlEnd) + 1;
    const braceOpen = source.indexOf('{', optionsStart);
    if (braceOpen < 0) continue;
    const options = objectExtent(source, braceOpen);
    if (!options) continue;
    const optionsInner = source.slice(options.start + 1, options.end - 1);
    if (!/body\s*:\s*JSON\.stringify\(/.test(optionsInner)) continue;

    const methodMatch = /method\s*:\s*['"](post|put|patch|delete)['"]/i.exec(optionsInner);
    if (!methodMatch) continue;
    const method = methodMatch[1].toLowerCase();
    const route = candidates.find((candidate) => candidate.method === method);
    if (!route) continue;

    const bodyCall = /body\s*:\s*JSON\.stringify\(\s*/.exec(optionsInner);
    if (!bodyCall) continue;
    const literalStart = options.start + 1 + bodyCall.index + bodyCall[0].length;
    if (source[literalStart] !== '{') {
      skippedPayloads += 1; // variable-built body: statically unresolvable
      continue;
    }
    const literal = objectExtent(source, literalStart);
    if (!literal) continue;
    const { keys, spread } = objectShape(source.slice(literal.start + 1, literal.end - 1));
    if (spread) skippedPayloads += 1;
    if (keys.length === 0) continue;

    const schema = requestSchemaFor(route.operation);
    if (!schema) continue;
    const allowed = new Set(Object.keys(schema.properties ?? {}));
    const unknown = keys.filter((key) => !allowed.has(key));
    checked += 1;
    if (unknown.length > 0) {
      const opId = route.operation.operationId ?? `${method} ${route.path}`;
      errors.push(
        `${relative(root, file)}: field(s) [${unknown.join(', ')}] not in request schema for ${method.toUpperCase()} ${route.path} (${opId})`
      );
    }
  }
}

walkFiles(frontendDir);

if (errors.length > 0) {
  console.error(`Frontend-v2 field guard: ${errors.length} static mismatch(es) found (${checked} payloads checked, ${skippedPayloads} dynamic bodies skipped).`);
  for (const error of errors) console.error(`  - ${error}`);
  process.exit(1);
}
console.log(`Frontend-v2 field guard: ok (${checked} static payload literal(s) matched to request schemas, ${skippedPayloads} dynamic bodies skipped).`);
