// Guard against hardcoded Tailwind palette colors creeping back into the
// frontend (WI-1318). Colors must go through --ds-* design tokens so a custom
// theme can recolor the whole UI; literal shades such as `text-blue-500` or
// `from-purple-400` bypass that system.
//
// Theme-neutral `white`/`black` utilities are intentionally allowed; only
// palette colors with a numeric shade are flagged.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sourceArgument = process.argv.indexOf('--src-dir');
const srcDir =
  sourceArgument === -1 ? path.join(root, 'src') : path.resolve(process.argv[sourceArgument + 1]);

const EXTENSIONS = new Set(['.svelte', '.js', '.ts', '.mjs', '.css']);
const PATTERN =
  /(?<![-\w])(?:bg|text|border|ring|from|to|via|fill|stroke|accent|divide|outline|decoration|placeholder|caret)-(?!opacity)[a-z]+-[0-9]{2,3}\b/g;

function walk(dir) {
  const files = [];
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) files.push(...walk(full));
    else if (EXTENSIONS.has(path.extname(entry))) files.push(full);
  }
  return files;
}

const violations = [];
const sources = walk(srcDir).map((file) => ({
  file,
  source: readFileSync(file, 'utf8').replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, ' ')
  ),
}));
const definitions = new Set();
for (const { source } of sources) {
  for (const match of source.matchAll(/(--ds-[\w-]+)\s*:/g)) definitions.add(match[1]);
  for (const match of source.matchAll(/setProperty\(\s*['"](--ds-[\w-]+)['"]/g))
    definitions.add(match[1]);
}
for (const { file, source } of sources) {
  for (const match of source.matchAll(PATTERN)) {
    const line = source.slice(0, match.index).split('\n').length;
    violations.push(`${path.relative(root, file)}:${line} ${match[0]}`);
  }
  // Match complete static names, not prefixes of template-built status/accent
  // families. Their resolved members must still be defined by the themes.
  for (const match of source.matchAll(/var\(\s*(--ds-[\w-]+)\s*(?=[,)])/g)) {
    if (definitions.has(match[1])) continue;
    const line = source.slice(0, match.index).split('\n').length;
    violations.push(`${path.relative(root, file)}:${line} ${match[1]}`);
  }
}

if (violations.length > 0) {
  console.error(
    'Design token guard failed. Define semantic tokens and avoid literal palette utilities:'
  );
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}

console.log('Hardcoded Tailwind color guard passed.');
console.log('Design token reference guard passed.');
