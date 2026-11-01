#!/usr/bin/env node

// Guards the audit taxonomy: action and resource names must come from the
// constants in internal/logger/audit.go, never from inline string literals.
// A raw literal silently drifts from the taxonomy and bypasses the review that
// adding a constant requires. The taxonomy file itself is exempt because that
// is where the constants are defined.

import fs from "node:fs";
import path from "node:path";

const roots = process.argv.slice(2).filter((argument) => !argument.startsWith("--"));
if (roots.length === 0) {
	roots.push("internal");
}

const taxonomySuffix = path.join("internal", "logger", "audit.go");

const files = [];
for (const root of roots) {
	walk(path.resolve(root));
}

function walk(entry) {
	const stat = fs.statSync(entry);
	if (stat.isDirectory()) {
		for (const child of fs.readdirSync(entry)) {
			if (child !== ".git" && child !== "node_modules") {
				walk(path.join(entry, child));
			}
		}
		return;
	}
	if (entry.endsWith(".go") && !entry.endsWith("_test.go")) {
		files.push(entry);
	}
}

// Audit call sites whose first two trailing arguments are action and resource.
const callNames = ["emitServiceAudit", "Log", "LogWithDetails", "LogFailure", "LogDenied"];

function findCallEnd(text, start) {
	let depth = 1;
	let quote = "";
	for (let index = start; index < text.length; index += 1) {
		const char = text[index];
		if (quote) {
			if (char === "\\") {
				index += 1;
			} else if (char === quote) {
				quote = "";
			}
			continue;
		}
		if (char === '"' || char === "'" || char === "`") {
			quote = char;
		} else if (char === "(") {
			depth += 1;
		} else if (char === ")") {
			depth -= 1;
			if (depth === 0) {
				return index;
			}
		}
	}
	return text.length;
}

function splitTopLevel(valueList) {
	const values = [];
	let start = 0;
	let depth = 0;
	let quote = "";
	for (let index = 0; index < valueList.length; index += 1) {
		const char = valueList[index];
		if (quote) {
			if (char === "\\") {
				index += 1;
			} else if (char === quote) {
				quote = "";
			}
			continue;
		}
		if (char === '"' || char === "'" || char === "`") {
			quote = char;
		} else if (char === "(" || char === "{" || char === "[") {
			depth += 1;
		} else if (char === ")" || char === "}" || char === "]") {
			depth -= 1;
		} else if (char === "," && depth === 0) {
			values.push(valueList.slice(start, index).trim());
			start = index + 1;
		}
	}
	if (valueList.slice(start).trim()) {
		values.push(valueList.slice(start).trim());
	}
	return values;
}

function isStringLiteral(value) {
	return value.length >= 2 && value.startsWith('"') && value.endsWith('"');
}

function lineOf(text, offset) {
	return text.slice(0, offset).split("\n").length;
}

function lineHasExemption(text, offset) {
	const start = text.lastIndexOf("\n", offset - 1) + 1;
	const end = text.indexOf("\n", offset);
	const line = text.slice(start, end === -1 ? undefined : end);
	return line.includes("audit-literal-ok");
}

const findings = [];

for (const file of files) {
	if (file.endsWith(taxonomySuffix)) {
		continue;
	}
	const text = fs.readFileSync(file, "utf8");
	const display = path.relative(process.cwd(), file);

	for (const name of callNames) {
		const pattern = new RegExp(`\\.${name}\\(`, "g");
		for (const match of text.matchAll(pattern)) {
			const open = match.index + match[0].length;
			const end = findCallEnd(text, open);
			const args = splitTopLevel(text.slice(open, end));
			if (args.length < 4) {
				continue;
			}
			for (const [index, label] of [
				[2, "action"],
				[3, "resource"],
			]) {
				if (isStringLiteral(args[index]) && !lineHasExemption(text, open)) {
					findings.push({
						file: display,
						line: lineOf(text, match.index),
						message: `raw ${label} literal ${args[index]} passed to ${name}; use a logger constant`,
					});
				}
			}
		}
	}

	// AuditEvent{ActionType: "...", ResourceType: "..."} literals.
	for (const match of text.matchAll(/AuditEvent\{/g)) {
		let depth = 1;
		let quote = "";
		let index = match.index + match[0].length;
		for (; index < text.length && depth > 0; index += 1) {
			const char = text[index];
			if (quote) {
				if (char === "\\") {
					index += 1;
				} else if (char === quote) {
					quote = "";
				}
				continue;
			}
			if (char === '"' || char === "'" || char === "`") {
				quote = char;
			} else if (char === "{") {
				depth += 1;
			} else if (char === "}") {
				depth -= 1;
			}
		}
		const body = text.slice(match.index, index);
		for (const [label, pattern] of [
			["action", /ActionType:\s*("[^"]*")/],
			["resource", /ResourceType:\s*("[^"]*")/],
		]) {
			const field = body.match(pattern);
			if (field && !lineHasExemption(text, match.index)) {
				findings.push({
					file: display,
					line: lineOf(text, match.index),
					message: `raw ${label} literal ${field[1]} passed to AuditEvent; use a logger constant`,
				});
			}
		}
	}
}

if (findings.length > 0) {
	for (const finding of findings.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line)) {
		console.error(`${finding.file}:${finding.line}: ${finding.message}`);
	}
	console.error(`Raw audit action/resource literals found: ${findings.length}`);
	process.exit(1);
}

console.log(`Audit literal check passed (${files.length} files).`);
