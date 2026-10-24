#!/usr/bin/env node
// Fails when the OpenUI component allow-list drifts between the renderer and
// the server validator.
//
// internal/customwidgets rejects a program that names a component it does not
// know, and frontend/src/lib/openui/library.js is what actually renders it. If
// the two lists disagree, a program the server accepts renders nothing (or a
// program the server rejects would have rendered fine). The lists are small
// literals, so reading them as text is reliable and avoids executing Svelte.

import { readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '../..');
const goFile = path.join(repoRoot, 'internal/customwidgets/customwidgets.go');
const jsFile = path.join(repoRoot, 'frontend/src/lib/openui/library.js');

function extractGoNames(source) {
  const start = source.indexOf('var componentNames = map[string]struct{}{');
  if (start === -1) throw new Error('componentNames map not found in customwidgets.go');
  const end = source.indexOf('\n}', start);
  if (end === -1) throw new Error('componentNames map is not terminated');
  const body = source.slice(start, end);
  return [...body.matchAll(/"([A-Za-z][A-Za-z0-9]*)":\s*\{\}/g)].map((match) => match[1]);
}

function extractJsNames(source) {
  const start = source.indexOf('export const componentNames = [');
  if (start === -1) throw new Error('componentNames array not found in library.js');
  const end = source.indexOf('];', start);
  if (end === -1) throw new Error('componentNames array is not terminated');
  const body = source.slice(start, end);
  return [...body.matchAll(/'([A-Za-z][A-Za-z0-9]*)'/g)].map((match) => match[1]);
}

async function main() {
  const [goSource, jsSource] = await Promise.all([
    readFile(goFile, 'utf8'),
    readFile(jsFile, 'utf8'),
  ]);

  const goNames = extractGoNames(goSource).sort();
  const jsNames = extractJsNames(jsSource).sort();

  const onlyGo = goNames.filter((name) => !jsNames.includes(name));
  const onlyJs = jsNames.filter((name) => !goNames.includes(name));

  if (onlyGo.length === 0 && onlyJs.length === 0) {
    console.log(`OpenUI component allow-list is in sync (${goNames.length} components).`);
    return;
  }

  console.error('OpenUI component allow-list drift:');
  if (onlyGo.length > 0) {
    console.error(`  accepted by the server but missing from the library: ${onlyGo.join(', ')}`);
  }
  if (onlyJs.length > 0) {
    console.error(`  in the library but rejected by the server: ${onlyJs.join(', ')}`);
  }
  console.error(
    '\nUpdate internal/customwidgets/customwidgets.go and frontend/src/lib/openui/library.js together.'
  );
  process.exitCode = 1;
}

await main();
