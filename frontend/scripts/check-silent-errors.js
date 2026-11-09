#!/usr/bin/env node
// Fails when a caught error is logged to the console but never surfaced to the
// user (no toast / surfaceError, no rethrow, no inline error state).
//
// This is a ratchet: `scripts/silent-error-baseline.json` records the silent
// scopes that already existed when the guard was introduced. The guard fails
// when a file grows past its baseline, or when a file with no baseline entry
// introduces one. Run with `--update` to rewrite the baseline after migrating
// sites to `surfaceError`.
//
// Surfacing is resolved through the call graph, so a catch that calls a local
// or imported wrapper (for example `showError` or `handleError`) counts as
// surfaced, not as a violation.

import { readdir, readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const srcDir = path.resolve(scriptDir, '../src');
const baselinePath = path.join(scriptDir, 'silent-error-baseline.json');
const update = process.argv.includes('--update');

const SURFACING_FNS = new Set([
  'errorToast',
  'warningToast',
  'infoToast',
  'successToast',
  'addToast',
  'surfaceError',
]);

const MODULE_SUFFIXES = ['.js', '.ts', '.svelte'];
const RESOLVE_SUFFIXES = ['', '.js', '.ts', '.svelte'];
const RESOLVE_INDEXES = ['index.js', 'index.ts', 'index.svelte'];

async function collectFiles(dir, out = []) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist') continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) await collectFiles(full, out);
    else if (MODULE_SUFFIXES.some((s) => entry.name.endsWith(s))) out.push(full);
  }
  return out;
}

function isTestFile(file) {
  return /\.(test|spec)\.[jt]s$/.test(file);
}

/** Extract the JS/TS body of a .svelte file, or the file itself. */
function scriptSegments(file, source) {
  if (!file.endsWith('.svelte')) return [{ code: source }];
  const segments = [];
  const re = /<script\b[^>]*>([\s\S]*?)<\/script>/g;
  let match;
  while ((match = re.exec(source))) segments.push({ code: match[1] });
  return segments;
}

async function resolveModule(fromFile, specifier) {
  if (!specifier.startsWith('.')) return null;
  const base = path.resolve(path.dirname(fromFile), specifier);
  const candidates = [
    ...RESOLVE_SUFFIXES.map((suffix) => `${base}${suffix}`),
    ...RESOLVE_INDEXES.map((index) => path.join(base, index)),
  ];
  for (const candidate of candidates) {
    try {
      if ((await import('node:fs')).statSync(candidate).isFile()) return candidate;
    } catch {
      // keep looking
    }
  }
  return null;
}

function callName(node) {
  const expr = node.expression;
  if (!expr) return null;
  if (ts.isIdentifier(expr)) return expr.text;
  if (ts.isPropertyAccessExpression(expr)) return expr.name.text;
  return null;
}

function isPropertyCall(node) {
  return ts.isPropertyAccessExpression(node.expression);
}

function collectCalls(node) {
  const calls = new Set();
  const walk = (child) => {
    if (ts.isCallExpression(child)) {
      const name = callName(child);
      if (name) calls.add(`${isPropertyCall(child) ? 'p' : 'i'}:${name}`);
    }
    ts.forEachChild(child, walk);
  };
  walk(node);
  return calls;
}

function hasExportModifier(node) {
  const modifiers = ts.canHaveModifiers(node) ? ts.getModifiers(node) : undefined;
  return !!modifiers?.some((m) => m.kind === ts.SyntaxKind.ExportKeyword);
}

/** Build the per-file function table, import map, and export map. */
async function parseFiles(files) {
  const fileInfo = new Map();
  for (const file of files) {
    const source = await readFile(file, 'utf8');
    const imports = new Map();
    const funcs = new Map();
    const exportsMap = new Map();

    for (const segment of scriptSegments(file, source)) {
      if (!segment.code.trim()) continue;
      const kind = file.endsWith('.svelte') ? ts.ScriptKind.TS : ts.ScriptKind.JS;
      const sf = ts.createSourceFile(file, segment.code, ts.ScriptTarget.Latest, true, kind);

      for (const stmt of sf.statements) {
        if (ts.isImportDeclaration(stmt) && stmt.importClause) {
          const target = await resolveModule(file, stmt.moduleSpecifier.text);
          const clause = stmt.importClause;
          if (clause.name) imports.set(clause.name.text, { target, name: 'default' });
          if (clause.namedBindings) {
            if (ts.isNamedImports(clause.namedBindings)) {
              for (const el of clause.namedBindings.elements) {
                imports.set(el.name.text, { target, name: (el.propertyName || el.name).text });
              }
            } else if (ts.isNamespaceImport(clause.namedBindings)) {
              imports.set(clause.namedBindings.name.text, { target, name: '*' });
            }
          }
        }
      }

      const register = (name, node, prefix = '') => {
        funcs.set(prefix + name, {
          key: `${file}::${prefix}${name}`,
          node,
          calls: collectCalls(node),
        });
      };

      const walkDecls = (node) => {
        if (ts.isFunctionDeclaration(node) && node.name) {
          register(node.name.text, node);
          if (hasExportModifier(node)) exportsMap.set(node.name.text, node.name.text);
        } else if (ts.isVariableStatement(node)) {
          const exported = hasExportModifier(node);
          for (const decl of node.declarationList.declarations) {
            if (
              ts.isIdentifier(decl.name) &&
              decl.initializer &&
              (ts.isArrowFunction(decl.initializer) || ts.isFunctionExpression(decl.initializer))
            ) {
              register(decl.name.text, decl.initializer);
              if (exported) exportsMap.set(decl.name.text, decl.name.text);
            }
          }
        } else if (ts.isClassDeclaration(node) && node.name) {
          for (const member of node.members) {
            if (
              (ts.isMethodDeclaration(member) ||
                ts.isGetAccessor(member) ||
                ts.isSetAccessor(member)) &&
              member.name &&
              ts.isIdentifier(member.name)
            ) {
              register(member.name.text, member, `${node.name.text}.`);
            }
          }
        }
        ts.forEachChild(node, walkDecls);
      };
      walkDecls(sf);

      for (const stmt of sf.statements) {
        if (
          ts.isExportDeclaration(stmt) &&
          stmt.exportClause &&
          ts.isNamedExports(stmt.exportClause)
        ) {
          for (const el of stmt.exportClause.elements) {
            exportsMap.set(el.name.text, (el.propertyName || el.name).text);
          }
        }
      }
    }

    fileInfo.set(file, { imports, funcs, exports: exportsMap });
  }
  return fileInfo;
}

function resolveCallee(file, rawName, fileInfo) {
  const [kind, name] = rawName.split(':');
  if (SURFACING_FNS.has(name)) return 'TOAST';
  const info = fileInfo.get(file);
  if (!info) return null;
  if (kind === 'i' && info.funcs.has(name)) return info.funcs.get(name).key;
  if (kind === 'i' && info.imports.has(name)) {
    const imported = info.imports.get(name);
    if (!imported.target || imported.name === '*') return null;
    const targetInfo = fileInfo.get(imported.target);
    if (!targetInfo) return null;
    const localName = targetInfo.exports.get(imported.name) ?? imported.name;
    if (targetInfo.funcs.has(localName)) return targetInfo.funcs.get(localName).key;
    return null;
  }
  // A method call `this.foo()` resolves to a same-file class method by name.
  if (kind === 'p' && info.funcs.has(name)) return info.funcs.get(name).key;
  return null;
}

function computeSurfaced(fileInfo) {
  const surfaced = new Set(['TOAST']);
  let changed = true;
  while (changed) {
    changed = false;
    for (const [file, info] of fileInfo) {
      for (const fn of info.funcs.values()) {
        if (surfaced.has(fn.key)) continue;
        for (const call of fn.calls) {
          const target = resolveCallee(file, call, fileInfo);
          if (target && (target === 'TOAST' || surfaced.has(target))) {
            surfaced.add(fn.key);
            changed = true;
            break;
          }
        }
      }
    }
  }
  return surfaced;
}

function scopeIsViolation(bodyNode, file, fileInfo, surfaced) {
  const text = bodyNode.getText();
  if (!/console\.(error|warn)/.test(text)) return false;
  if (/\bthrow\b/.test(text)) return false;
  // An inline error state is a surface too (`error = ...`, `formErrors = ...`,
  // `state.error = ...`).
  if (/\w*[Ee]rror\w*\s*=[^=]/.test(text)) return false;
  for (const call of collectCalls(bodyNode)) {
    const target = resolveCallee(file, call, fileInfo);
    if (target === 'TOAST' || (target && surfaced.has(target))) return false;
  }
  return true;
}

function findViolations(file, sf, fileInfo, surfaced, baseLine) {
  const violations = [];
  const visit = (node) => {
    if (ts.isCatchClause(node) && scopeIsViolation(node.block, file, fileInfo, surfaced)) {
      const lc = sf.getLineAndCharacterOfPosition(node.getStart());
      violations.push(lc.line + 1 + baseLine);
    } else if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === 'catch' &&
      node.arguments.length &&
      scopeIsViolation(node.arguments[0], file, fileInfo, surfaced)
    ) {
      const lc = sf.getLineAndCharacterOfPosition(node.getStart());
      violations.push(lc.line + 1 + baseLine);
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return violations;
}

async function countViolations(files, fileInfo, surfaced, baseDir) {
  const counts = new Map();
  for (const file of files) {
    const source = await readFile(file, 'utf8');
    const rel = path.relative(baseDir, file);
    let count = 0;
    for (const segment of scriptSegments(file, source)) {
      if (!segment.code.trim()) continue;
      const kind = file.endsWith('.svelte') ? ts.ScriptKind.TS : ts.ScriptKind.JS;
      const sf = ts.createSourceFile(file, segment.code, ts.ScriptTarget.Latest, true, kind);
      count += findViolations(file, sf, fileInfo, surfaced, 0).length;
    }
    if (count > 0) counts.set(rel, count);
  }
  return counts;
}

/**
 * Analyze files and return a `{ relativePath: silentScopeCount }` map.
 * @param {string[]} files absolute paths
 * @param {string} baseDir directory the relative keys are computed against
 */
export async function countSilentScopes(files, baseDir) {
  const fileInfo = await parseFiles(files);
  const surfaced = computeSurfaced(fileInfo);
  return countViolations(files, fileInfo, surfaced, baseDir);
}

/**
 * @param {Map<string, number>} counts
 * @param {Record<string, number>} baseline
 * @returns {{ file: string, count: number, allowed: number }[]}
 */
export function findRegressions(counts, baseline) {
  const regressions = [];
  for (const [file, count] of counts) {
    const allowed = baseline[file] ?? 0;
    if (count > allowed) regressions.push({ file, count, allowed });
  }
  return regressions;
}

async function readBaseline() {
  try {
    return JSON.parse(await readFile(baselinePath, 'utf8'));
  } catch {
    return {};
  }
}

async function main() {
  const allFiles = await collectFiles(srcDir);
  const files = allFiles.filter((file) => !isTestFile(file));
  const counts = await countSilentScopes(files, srcDir);
  const current = Object.fromEntries([...counts.entries()].sort());

  if (update) {
    await writeFile(baselinePath, `${JSON.stringify(current, null, 2)}\n`);
    const total = [...counts.values()].reduce((a, b) => a + b, 0);
    console.log(
      `Silent-error baseline updated: ${Object.keys(current).length} files, ${total} scopes.`
    );
    return;
  }

  const baseline = await readBaseline();
  const regressions = findRegressions(counts, baseline);

  const total = [...counts.values()].reduce((a, b) => a + b, 0);
  if (regressions.length > 0) {
    console.error(
      'Caught errors that only log to the console were added. Surface them with\n' +
        '`surfaceError(error, { context })`, rethrow, or opt out with\n' +
        '`surfaceError(error, { silent: true })` for background work:\n'
    );
    for (const { file, count, allowed } of regressions.sort((a, b) => b.count - a.count)) {
      console.error(`  ${file}: ${count} silent scope(s) (baseline ${allowed})`);
    }
    console.error(
      `\n${regressions.length} file(s) regressed. If the new sites are intentional,` +
        ' run `node scripts/check-silent-errors.js --update` in the same change.'
    );
    process.exitCode = 1;
    return;
  }

  console.log(
    `Silent-error guard passed (${total} known silent scope(s) in ` +
      `${Object.keys(baseline).length} file(s); none added).`
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main();
}
