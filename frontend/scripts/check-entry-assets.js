#!/usr/bin/env node
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultIndexPath = path.resolve(scriptDir, '../dist/index.html');

const forbiddenEntryAssets = [
  { name: 'Excalidraw', pattern: /(?:excalidraw|mermaid-to-excalidraw)/i },
  { name: 'Mermaid', pattern: /(?:^|[/_.-])mermaid(?:[/_.-]|$)/i },
  { name: 'Cytoscape', pattern: /(?:^|[/_.-])cytoscape(?:[/_.-]|$)/i },
  { name: 'React', pattern: /(?:^|[/_.-])react(?:-dom)?(?:[/_.-]|$)/i },
  { name: 'SvelteFlow', pattern: /(?:svelteflow|xyflow)/i },
  { name: 'D3', pattern: /(?:^|[/_.-])d3(?:[/_.-]|$)/i },
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
 * Compare the emitted root App chunk against its regression budget.
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
  const rootChunkNames = (await readdir(assetsDir)).filter((name) => /^App-[^.]+\.js$/.test(name));
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

  console.log(
    `Entry asset check passed: optional route/graph dependencies remain lazy; root shell ${formatKiB(rootAsset.rawBytes)} raw / ${formatKiB(rootAsset.gzipBytes)} gzip.`
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
}
