#!/usr/bin/env node
import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(scriptDir, '..');
const srcDir = path.join(rootDir, 'src');
const baselinePath = path.join(rootDir, 'scripts', 'shortcut-baseline.json');
const exemptionMarker = 'shortcut-guard-exempt';

function normalizeSnippet(value) {
  return value.replace(/\s+/g, ' ').trim().slice(0, 220);
}

function lineFor(source, index) {
  return source.slice(0, index).split('\n').length;
}

function hasAttr(attrs, name) {
  return new RegExp(`(?:^|\\s)${name}(?:\\s|=|$)`).test(attrs);
}

function hasRecentExemption(source, index) {
  const recent = source.slice(Math.max(0, index - 260), index);
  return recent.includes(exemptionMarker);
}

async function listSvelteFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    if (entry.name === 'node_modules' || entry.name === 'dist') continue;

    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listSvelteFiles(fullPath)));
    } else if (entry.isFile() && entry.name.endsWith('.svelte')) {
      files.push(fullPath);
    }
  }

  return files;
}

function pushIssue(issues, source, file, index, rule, message, detail) {
  issues.push({
    rule,
    file,
    line: lineFor(source, index),
    message,
    detail,
    signature: `${file}:${rule}:${detail}`,
  });
}

function findModalRanges(source) {
  const ranges = [];
  const modalPattern = /<Modal(?:Backdrop)?\b[\s\S]*?<\/Modal(?:Backdrop)?>/g;
  let match;

  while ((match = modalPattern.exec(source)) !== null) {
    ranges.push([match.index, match.index + match[0].length]);
  }

  return ranges;
}

function isInsideAnyRange(index, ranges) {
  return ranges.some(([start, end]) => index >= start && index <= end);
}

function checkModalSubmitShortcuts(source, file, issues) {
  const modalPattern = /<Modal\b([^>]*)>([\s\S]*?)<\/Modal>/g;
  let match;

  while ((match = modalPattern.exec(source)) !== null) {
    const [block, attrs, body] = match;
    if (block.includes(exemptionMarker) || hasRecentExemption(source, match.index)) continue;

    const footerMatches = [...body.matchAll(/<DialogFooter\b([^>]*)/g)];
    const hasConfirmFooter = footerMatches.some((footerMatch) =>
      hasAttr(footerMatch[1], 'onConfirm')
    );

    if (hasConfirmFooter && !hasAttr(attrs, 'onSubmit')) {
      pushIssue(
        issues,
        source,
        file,
        match.index,
        'modal-confirm-without-submit-shortcut',
        'Modal contains a confirming DialogFooter but does not pass onSubmit, so Enter/Cmd+Enter will not submit.',
        normalizeSnippet(`<Modal${attrs}>`)
      );
    }

    for (const footerMatch of footerMatches) {
      const footerAttrs = footerMatch[1];
      if (!hasAttr(footerAttrs, 'onConfirm')) continue;
      if (hasAttr(footerAttrs, 'showKeyboardHint')) continue;

      pushIssue(
        issues,
        source,
        file,
        match.index + footerMatch.index,
        'dialog-footer-confirm-without-keyboard-hint',
        'DialogFooter has a confirm action without visible keyboard hints.',
        normalizeSnippet(`<DialogFooter${footerAttrs}>`)
      );
    }
  }
}

function checkModalBackdropConfirmShortcuts(source, file, issues) {
  const backdropPattern = /<ModalBackdrop\b([^>]*)>([\s\S]*?)<\/ModalBackdrop>/g;
  let match;

  while ((match = backdropPattern.exec(source)) !== null) {
    const [block, attrs, body] = match;
    if (block.includes(exemptionMarker) || hasRecentExemption(source, match.index)) continue;

    const hasPrimaryButton =
      /<Button\b[^>]*variant=(?:"primary"|'primary'|{['"]primary['"]})/s.test(body);
    const hasSubmitHandler =
      /onkeydown=|on:keydown|matchesShortcut|hotkeyConfig|type=(?:"submit"|'submit'|{['"]submit['"]})/s.test(
        block
      );

    if (hasPrimaryButton && !hasSubmitHandler) {
      pushIssue(
        issues,
        source,
        file,
        match.index,
        'modal-backdrop-confirm-without-shortcut',
        'ModalBackdrop contains a primary action but no obvious submit shortcut handling.',
        normalizeSnippet(`<ModalBackdrop${attrs}>`)
      );
    }
  }
}

function checkAddCreateButtonShortcuts(source, file, issues) {
  const modalRanges = findModalRanges(source);
  const buttonPattern = /<Button\b([^>]*)>([\s\S]*?)<\/Button>/g;
  let match;

  while ((match = buttonPattern.exec(source)) !== null) {
    const [block, attrs, body] = match;
    if (block.includes(exemptionMarker) || hasRecentExemption(source, match.index)) continue;
    if (isInsideAnyRange(match.index, modalRanges)) continue;
    if (!hasAttr(attrs, 'onclick')) continue;

    const candidate =
      /icon=\{(?:Icon)?Plus\}/.test(attrs) ||
      /\b(add|create|new)[A-Z][A-Za-z0-9_]*/.test(attrs) ||
      /\b(add|create|new)(?:\s|<|\{|\(|\.|$)/i.test(body);

    if (!candidate) continue;
    if (hasAttr(attrs, 'keyboardHint') && hasAttr(attrs, 'hotkeyConfig')) continue;

    pushIssue(
      issues,
      source,
      file,
      match.index,
      'add-create-button-without-shortcut',
      'Likely Add/Create/New button is missing a visible keyboard hint and hotkeyConfig.',
      normalizeSnippet(`<Button${attrs}>${body}`)
    );
  }
}

async function collectIssues() {
  const files = await listSvelteFiles(srcDir);
  const issues = [];

  for (const fullPath of files) {
    const source = await readFile(fullPath, 'utf8');
    // Baseline signatures use repository paths on every platform.
    const file = path.relative(rootDir, fullPath).split(path.sep).join('/');

    checkModalSubmitShortcuts(source, file, issues);
    checkModalBackdropConfirmShortcuts(source, file, issues);
    checkAddCreateButtonShortcuts(source, file, issues);
  }

  return issues.sort(
    (a, b) => a.file.localeCompare(b.file) || a.line - b.line || a.rule.localeCompare(b.rule)
  );
}

function countsBySignature(issues) {
  const counts = new Map();
  for (const issue of issues) {
    counts.set(issue.signature, (counts.get(issue.signature) ?? 0) + 1);
  }
  return counts;
}

async function readBaseline() {
  try {
    const baseline = JSON.parse(await readFile(baselinePath, 'utf8'));
    return new Map((baseline.violations ?? []).map((entry) => [entry.signature, entry.count]));
  } catch (error) {
    if (error.code === 'ENOENT') return new Map();
    throw error;
  }
}

async function writeBaseline(issues) {
  const counts = countsBySignature(issues);
  const examples = new Map();
  for (const issue of issues) {
    if (!examples.has(issue.signature)) examples.set(issue.signature, issue);
  }

  const violations = [...counts.entries()]
    .map(([signature, count]) => {
      const issue = examples.get(signature);
      return {
        signature,
        count,
        file: issue.file,
        rule: issue.rule,
        message: issue.message,
      };
    })
    .sort((a, b) => a.signature.localeCompare(b.signature));

  await writeFile(baselinePath, `${JSON.stringify({ version: 1, violations }, null, 2)}\n`);
}

function printIssue(issue) {
  console.error(`${issue.file}:${issue.line}: ${issue.rule}`);
  console.error(`  ${issue.message}`);
}

const issues = await collectIssues();

if (process.env.UPDATE_SHORTCUT_BASELINE === '1') {
  await writeBaseline(issues);
  console.log(`Updated shortcut baseline with ${issues.length} existing issue(s).`);
  process.exit(0);
}

const baseline = await readBaseline();
const currentCounts = countsBySignature(issues);
const newIssues = issues.filter(
  (issue) => currentCounts.get(issue.signature) > (baseline.get(issue.signature) ?? 0)
);

if (newIssues.length > 0) {
  console.error('Shortcut guard found new dialog/button shortcut violations:');
  for (const issue of newIssues) printIssue(issue);
  console.error(
    `\nFix the shortcut behavior or add a nearby <!-- ${exemptionMarker}: reason --> comment.`
  );
  console.error(
    'If you intentionally backfilled the current state, run UPDATE_SHORTCUT_BASELINE=1 node scripts/check-shortcuts.js.'
  );
  process.exit(1);
}

console.log(`Shortcut guard passed (${issues.length} existing issue(s), no new violations).`);
