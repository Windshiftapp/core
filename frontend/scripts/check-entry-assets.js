#!/usr/bin/env node
import { readFileSync } from 'node:fs';
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultIndexPath = path.resolve(scriptDir, '../dist/index.html');

// Large optional libraries that must stay behind dynamic imports. The desktop
// shell imports the shared sanitizer statically, so a chunking mistake can drag
// an editor and its dependencies into the cold-start closure.
const optionalLibraryAssets = [
  { name: 'Excalidraw', pattern: /(?:excalidraw|mermaid-to-excalidraw)/i },
  { name: 'Mermaid', pattern: /(?:^|[/_.-])mermaid(?:[/_.-]|$)/i },
  { name: 'Cytoscape', pattern: /(?:^|[/_.-])cytoscape(?:[/_.-]|$)/i },
  { name: 'Milkdown', pattern: /(?:^|[/_.-])milkdown(?:[/_.-]|$)/i },
  { name: 'React', pattern: /(?:^|[/_.-])react(?:-dom)?(?:[/_.-]|$)/i },
  { name: 'SvelteFlow', pattern: /(?:svelteflow|xyflow)/i },
  { name: 'D3', pattern: /(?:^|[/_.-])d3(?:[/_.-]|$)/i },
];

const forbiddenEntryAssets = [
  ...optionalLibraryAssets,
  { name: 'desktop shell', pattern: /(?:^|\/)MainApp-/i },
  { name: 'mobile shell', pattern: /(?:^|\/)MobileShell-/i },
  { name: 'login dialog', pattern: /(?:^|\/)LoginDialog-/i },
  { name: 'setup assistant', pattern: /(?:^|\/)WelcomeAssistant-/i },
  { name: 'portal shell', pattern: /(?:^|\/)Portal-/i },
  { name: 'public form', pattern: /(?:^|\/)PublicFormPage-/i },
  { name: 'public board', pattern: /(?:^|\/)PublicBoard-/i },
  { name: 'print view', pattern: /(?:^|\/)PagePrintView-/i },
  { name: 'password setup', pattern: /(?:^|\/)SetPassword-/i },
];

export const ROOT_SHELL_BUDGET = Object.freeze({
  rawBytes: 50 * 1024,
  gzipBytes: 20 * 1024,
});

// The desktop shell closure is the static JS the authenticated shell must load
// before it can mount. It excludes the selected route and dynamic translations.
// The budget leaves headroom for normal growth while still catching a leaked
// optional editor (before the Milkdown fix the closure was ~1,546 KB raw /
// ~524 KB gzip).
export const DESKTOP_SHELL_CLOSURE_BUDGET = Object.freeze({
  rawBytes: 1_310_720,
  gzipBytes: 440_000,
});

/**
 * Return optional feature assets that index.html loads before route-level
 * dynamic imports have a chance to run.
 *
 * @param {string} html
 * @returns {{ name: string, asset: string }[]}
 */
export function findForbiddenEntryAssets(html) {
  const assets = new Set();
  const assetPattern = /<(?:script|link)\b[^>]*?\b(?:src|href)=["']([^"']+)["'][^>]*>/gi;
  let match;

  while ((match = assetPattern.exec(html)) !== null) {
    assets.add(match[1]);
  }

  const violations = [];
  for (const asset of assets) {
    for (const forbidden of forbiddenEntryAssets) {
      if (forbidden.pattern.test(asset)) {
        violations.push({ name: forbidden.name, asset });
      }
    }
  }
  return violations;
}

/**
 * Extract static import and re-export specifiers from an emitted chunk.
 * Dynamic `import()` calls are intentionally not matched.
 *
 * @param {string} source
 * @returns {string[]}
 */
export function findStaticImports(source) {
  const specifiers = new Set();
  const patterns = [
    /\bimport\s*(?:[\w*{},\s]+?\s*from\s*)?["']([^"']+)["']/g,
    /\bexport\s*(?:[\w*{},\s]+?\s*)?from\s*["']([^"']+)["']/g,
  ];

  for (const pattern of patterns) {
    let match;
    while ((match = pattern.exec(source)) !== null) {
      specifiers.add(match[1]);
    }
  }
  return [...specifiers];
}

/**
 * Follow static imports from the given entry assets. Dynamic imports are not
 * followed, so the result is the cold-start closure for those entries. Missing
 * assets are skipped rather than aborting the walk.
 *
 * @param {string[]} entryAssets
 * @param {(asset: string) => string} readAsset
 * @returns {string[]}
 */
export function collectStaticClosure(entryAssets, readAsset) {
  const visited = new Set();
  const pending = [...entryAssets];

  while (pending.length > 0) {
    const asset = pending.pop();
    if (visited.has(asset)) continue;

    let source;
    try {
      source = readAsset(asset);
    } catch {
      // A missing asset contributes no bytes and no further imports.
      continue;
    }
    visited.add(asset);

    for (const specifier of findStaticImports(source)) {
      if (specifier.startsWith('./')) {
        pending.push(specifier);
      }
    }
  }
  return [...visited];
}

/**
 * Return optional feature libraries that leaked into a static closure.
 *
 * @param {string[]} assets
 * @returns {{ name: string, asset: string }[]}
 */
export function findForbiddenClosureAssets(assets) {
  const violations = [];
  for (const asset of assets) {
    for (const forbidden of optionalLibraryAssets) {
      if (forbidden.pattern.test(asset)) {
        violations.push({ name: forbidden.name, asset });
      }
    }
  }
  return violations;
}

/**
 * Compare an emitted chunk or closure total against a regression budget.
 *
 * @param {{ name: string, rawBytes: number, gzipBytes: number }} asset
 * @param {{ rawBytes: number, gzipBytes: number }} [budget]
 * @returns {{ metric: 'raw'|'gzip', actual: number, limit: number }[]}
 */
export function findRootShellBudgetViolations(asset, budget = ROOT_SHELL_BUDGET) {
  const violations = [];
  if (asset.rawBytes > budget.rawBytes) {
    violations.push({ metric: 'raw', actual: asset.rawBytes, limit: budget.rawBytes });
  }
  if (asset.gzipBytes > budget.gzipBytes) {
    violations.push({ metric: 'gzip', actual: asset.gzipBytes, limit: budget.gzipBytes });
  }
  return violations;
}

function formatKiB(bytes) {
  return `${(bytes / 1024).toFixed(2)} KiB`;
}

async function main() {
  const indexPath = process.argv[2] ? path.resolve(process.argv[2]) : defaultIndexPath;
  const html = await readFile(indexPath, 'utf8');
  const violations = findForbiddenEntryAssets(html);

  if (violations.length > 0) {
    console.error('Optional feature assets leaked into the initial application entry:');
    for (const violation of violations) {
      console.error(`- ${violation.name}: ${violation.asset}`);
    }
    process.exitCode = 1;
    return;
  }

  const assetsDir = path.join(path.dirname(indexPath), '_app');
  const assetNames = await readdir(assetsDir);
  const readAsset = (asset) =>
    readFileSync(path.join(assetsDir, asset.replace(/^\.\//, '')), 'utf8');

  const rootChunkNames = assetNames.filter((name) => /^App-[^.]+\.js$/.test(name));
  if (rootChunkNames.length !== 1) {
    console.error(
      `Expected exactly one emitted root App chunk, found ${rootChunkNames.length}: ${rootChunkNames.join(', ')}`
    );
    process.exitCode = 1;
    return;
  }

  const rootChunkName = rootChunkNames[0];
  const rootChunk = await readFile(path.join(assetsDir, rootChunkName));
  const rootAsset = {
    name: rootChunkName,
    rawBytes: rootChunk.byteLength,
    gzipBytes: gzipSync(rootChunk).byteLength,
  };
  const budgetViolations = findRootShellBudgetViolations(rootAsset);
  if (budgetViolations.length > 0) {
    console.error(`Root application shell exceeded its bundle budget (${rootAsset.name}):`);
    for (const violation of budgetViolations) {
      console.error(
        `- ${violation.metric}: ${formatKiB(violation.actual)} > ${formatKiB(violation.limit)}`
      );
    }
    process.exitCode = 1;
    return;
  }

  const shellChunkNames = assetNames.filter((name) => /^MainApp-[^.]+\.js$/.test(name));
  if (shellChunkNames.length !== 1) {
    console.error(
      `Expected exactly one emitted desktop shell chunk, found ${shellChunkNames.length}: ${shellChunkNames.join(', ')}`
    );
    process.exitCode = 1;
    return;
  }

  const shellEntry = `./${shellChunkNames[0]}`;
  const closure = collectStaticClosure([shellEntry], readAsset);
  const closureViolations = findForbiddenClosureAssets(closure);
  if (closureViolations.length > 0) {
    console.error('Optional feature assets leaked into the desktop shell static closure:');
    for (const violation of closureViolations) {
      console.error(`- ${violation.name}: ${violation.asset}`);
    }
    process.exitCode = 1;
    return;
  }

  let closureRaw = 0;
  let closureGzip = 0;
  for (const asset of closure) {
    const content = await readFile(path.join(assetsDir, asset.replace(/^\.\//, '')));
    closureRaw += content.byteLength;
    closureGzip += gzipSync(content).byteLength;
  }
  const closureAsset = {
    name: `${shellEntry} closure`,
    rawBytes: closureRaw,
    gzipBytes: closureGzip,
  };
  const closureBudgetViolations = findRootShellBudgetViolations(
    closureAsset,
    DESKTOP_SHELL_CLOSURE_BUDGET
  );
  if (closureBudgetViolations.length > 0) {
    console.error(`Desktop shell closure exceeded its bundle budget (${closureAsset.name}):`);
    for (const violation of closureBudgetViolations) {
      console.error(
        `- ${violation.metric}: ${formatKiB(violation.actual)} > ${formatKiB(violation.limit)}`
      );
    }
    process.exitCode = 1;
    return;
  }

  console.log(
    `Entry asset check passed: optional route/graph dependencies remain lazy; root shell ${formatKiB(rootAsset.rawBytes)} raw / ${formatKiB(rootAsset.gzipBytes)} gzip; desktop shell closure ${closure.length} chunks, ${formatKiB(closureRaw)} raw / ${formatKiB(closureGzip)} gzip.`
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
}
