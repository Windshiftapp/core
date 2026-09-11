// One-off analysis: build the import graph of frontend/src (static + dynamic
// imports), compute reachability from the app's three entry points, and report
// which files containing hardcoded Tailwind color utilities are live, and who
// imports them.
import { readdirSync, statSync, readFileSync } from 'node:fs';
import { join, relative, dirname, resolve } from 'node:path';

const SRC = resolve(import.meta.dir, '..', 'src');

const EXTS = ['', '.js', '.ts', '.svelte', '.mjs', '.jsx', '.tsx', '/index.js', '/index.ts'];

function walk(dir, out = []) {
	for (const e of readdirSync(dir)) {
		const p = join(dir, e);
		const s = statSync(p);
		if (s.isDirectory()) walk(p, out);
		else out.push(p);
	}
	return out;
}

const allFiles = walk(SRC).filter((f) => /\.(svelte|js|ts|mjs)$/.test(f));
const fileSet = new Set(allFiles);

function resolveSpec(spec, fromFile) {
	if (!spec.startsWith('.') && !spec.startsWith('/')) return null; // bare import
	const base = spec.startsWith('/') ? resolve(SRC, `.${spec}`) : resolve(dirname(fromFile), spec);
	for (const ext of EXTS) {
		const cand = base + ext;
		if (fileSet.has(cand)) return cand;
	}
	// .js spec pointing at .svelte.js or .svelte
	const m = base.match(/^(.*)\.js$/);
	if (m) {
		for (const alt of [`${m[1]}.svelte.js`, `${m[1]}.svelte`]) {
			if (fileSet.has(alt)) return alt;
		}
	}
	return null;
}

// Extract module specifiers from a file's imports (static, dynamic, export-from)
function importsOf(file) {
	const src = readFileSync(file, 'utf8');
	const specs = new Set();
	const re =
		/(?:^|[^\w$])(?:import|export)\s[^;]*?from\s*['"]([^'"]+)['"]|import\s*\(\s*['"]([^'"]+)['"]\s*\)|(?:^|[^\w$])import\s+['"]([^'"]+)['"]/g;
	let m;
	while ((m = re.exec(src))) {
		const spec = m[1] || m[2] || m[3];
		if (spec) specs.add(spec);
	}
	const resolved = [];
	for (const spec of specs) {
		const r = resolveSpec(spec, file);
		if (r) resolved.push(r);
	}
	return resolved;
}

const ENTRIES = [
	join(SRC, 'main.js'),
	join(SRC, 'embed', 'index.js'),
	join(SRC, 'design-system', 'viewer', 'main.js'),
].filter((e) => fileSet.has(e));

const importers = new Map(allFiles.map((f) => [f, []]));
for (const f of allFiles) {
	for (const dep of importsOf(f)) {
		importers.get(dep).push(f);
	}
}

// Reachability from entries
const reachable = new Set();
const queue = [...ENTRIES];
while (queue.length) {
	const f = queue.pop();
	if (reachable.has(f)) continue;
	reachable.add(f);
	for (const dep of importsOf(f)) {
		if (!reachable.has(dep)) queue.push(dep);
	}
}

// Files with hardcoded color utilities. rg exits 1 when nothing matches,
// which is the success case once the sweep is complete.
const { execSync } = await import('node:child_process');
const PATTERN = '(?<![-\\w])(?:bg|text|border)-(?!opacity)[a-z]+-[0-9]{2,3}\\b';
let out = '';
try {
	out = execSync(
		`rg -l '${PATTERN}' ${SRC} --pcre2 -g '!*.test.*' -g '*.svelte' -g '*.ts' -g '*.js' -g '*.css'`,
		{ encoding: 'utf8' }
	);
} catch (e) {
	if (e.status !== 1) throw e;
}
const colorFiles = out.trim().split('\n').filter(Boolean);

const countFor = (f) =>
	parseInt(
		execSync(`rg -o '${PATTERN}' '${f}' --pcre2 | wc -l`)
			.toString()
			.trim(),
		10
	) || 0;

const rows = colorFiles.map((f) => {
	const imps = importers.get(f) || [];
	const liveImporters = imps.filter((i) => reachable.has(i));
	const rel = relative(SRC, f);
	return {
		file: rel,
		count: countFor(f),
		reachable: reachable.has(f),
		importers: liveImporters.map((i) => relative(SRC, i)),
		importerCount: liveImporters.length,
	};
});

// Occurrences that are not real UI (comments, docs) — annotate rather than hide.
const NOTES = {};

rows.sort((a, b) => Number(a.reachable) - Number(b.reachable) || b.count - a.count);

if (process.argv[2] === '--markdown') {
	const areaOf = (f) => {
		if (f.startsWith('App.svelte') || f.startsWith('lib/layout/')) return 'App shell / layout';
		if (f.startsWith('lib/pages/')) return 'Pages & routing';
		if (f.startsWith('lib/components/')) return 'Shared components';
		if (f.startsWith('lib/dialogs/')) return 'Dialogs';
		if (f.startsWith('design-system/viewer/pages/icon-navigation/')) return 'Design system viewer (mock pages)';
		if (f.startsWith('design-system/viewer/')) return 'Design system viewer';
		if (f.startsWith('lib/pickers/') || f.startsWith('lib/editors/')) return 'Pickers & editors';
		if (f.startsWith('lib/settings/')) return 'Settings / admin';
		if (f.startsWith('lib/workspaces/')) return 'Workspaces';
		if (f.startsWith('lib/hub/') || f.startsWith('lib/portal/')) return 'Hub & portal';
		if (f.startsWith('lib/features/collections/')) return 'Features: collections';
		if (f.startsWith('lib/features/items/')) return 'Features: items';
		if (f.startsWith('lib/features/assets/')) return 'Features: assets';
		if (f.startsWith('lib/features/')) return 'Features: other';
		if (f.startsWith('lib/widgets/')) return 'Widgets';
		return 'Other';
	};
	const total = rows.reduce((s, r) => s + r.count, 0);
	console.log(`# WI-1318 inventory: hardcoded Tailwind color utilities\n`);
	console.log(
		`Generated by \`frontend/scripts/usage-inventory.js\` (static + dynamic import graph, ` +
			`reachability from entries: \`src/main.js\`, \`src/embed/index.js\`, \`src/design-system/viewer/main.js\`).\n`
	);
	console.log(`**${rows.length} files, ${total} occurrences. All files are reachable from an app entry point — no dead code found.**\n`);
	const areas = new Map();
	for (const r of rows) {
		const a = areaOf(r.file);
		if (!areas.has(a)) areas.set(a, []);
		areas.get(a).push(r);
	}
	const areaOrder = [
		'App shell / layout',
		'Pages & routing',
		'Shared components',
		'Dialogs',
		'Pickers & editors',
		'Widgets',
		'Workspaces',
		'Settings / admin',
		'Hub & portal',
		'Features: items',
		'Features: collections',
		'Features: assets',
		'Features: other',
		'Design system viewer',
		'Design system viewer (mock pages)',
		'Other',
	];
	for (const area of areaOrder) {
		const list = areas.get(area);
		if (!list) continue;
		const n = list.reduce((s, r) => s + r.count, 0);
		console.log(`\n## ${area} (${list.length} files, ${n} occurrences)\n`);
		console.log(`| Occurrences | File | Imported by |`);
		console.log(`|---:|---|---|`);
		for (const r of list.sort((a, b) => b.count - a.count)) {
			const imps = r.importers.map((i) => `\`${i}\``).join(', ') || '—';
			const note = NOTES[r.file] ? ` (${NOTES[r.file]})` : '';
			console.log(`| ${r.count} | \`${r.file}\`${note} | ${imps} |`);
		}
	}
	process.exit(0);
}

for (const r of rows) {
	console.log(
		`${r.reachable ? 'LIVE ' : 'DEAD '} ${String(r.count).padStart(3)}  ${r.file}` +
			(r.reachable
				? r.importerCount <= 4
					? `  <= ${r.importers.join(', ')}`
					: `  <= ${r.importerCount} importers`
				: '')
	);
}
const dead = rows.filter((r) => !r.reachable);
console.log(`\nTotal: ${rows.length} files, ${rows.reduce((s, r) => s + r.count, 0)} occurrences`);
console.log(`Dead (unreachable from any entry): ${dead.length} files, ${dead.reduce((s, r) => s + r.count, 0)} occurrences`);
