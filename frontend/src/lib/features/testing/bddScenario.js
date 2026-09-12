// Client-side helpers for the BDD test case format. The parsed scenario
// spec (gherkin.ScenarioSpec JSON) is produced by the backend; this module
// only expands example values and shapes execution views.

/**
 * Replace every <name> placeholder whose name exists in values with the row
 * value. Unknown placeholders stay visible. Replacement values are not
 * rescanned, mirroring the backend expander.
 */
export function expandText(text, values) {
  if (!text || !values || Object.keys(values).length === 0 || !text.includes('<')) {
    return text;
  }
  let out = '';
  let rest = text;
  for (;;) {
    const start = rest.indexOf('<');
    if (start < 0) {
      out += rest;
      return out;
    }
    const end = rest.indexOf('>', start);
    if (end < 0) {
      out += rest;
      return out;
    }
    const name = rest.slice(start + 1, end);
    out += rest.slice(0, start);
    if (Object.hasOwn(values, name)) {
      out += values[name];
    } else {
      out += rest.slice(start, end + 1);
    }
    rest = rest.slice(end + 1);
  }
}

/** Expand one authored step (keyword, text, table, doc string) for a row. */
export function expandStep(step, values) {
  const expanded = { keyword: step.keyword, text: expandText(step.text, values) };
  if (step.data_table) {
    expanded.data_table = {
      rows: step.data_table.rows.map((row) => row.map((cell) => expandText(cell, values))),
    };
  }
  if (step.doc_string) {
    expanded.doc_string = {
      content_type: expandText(step.doc_string.content_type ?? '', values),
      content: expandText(step.doc_string.content ?? '', values),
    };
  }
  return expanded;
}

/**
 * Concrete steps for one execution of a BDD case: Background steps first,
 * then the scenario's own steps, with the row values substituted.
 */
export function executionSteps(spec, values) {
  const steps = [];
  for (const step of spec.background ?? []) {
    steps.push(expandStep(step, values));
  }
  for (const step of spec.steps ?? []) {
    steps.push(expandStep(step, values));
  }
  return steps;
}

/**
 * Flattened execution contexts of a BDD case: one per Examples row for a
 * Scenario Outline, or a single implicit context for a plain Scenario.
 * Each context carries its 0-based index and input values.
 */
export function exampleContexts(spec) {
  const contexts = [];
  let index = 0;
  for (const block of spec?.examples ?? []) {
    for (const row of block.rows ?? []) {
      const values = {};
      (block.header ?? []).forEach((column, i) => {
        if (i < row.length) values[column] = row[i];
      });
      contexts.push({ index, block, values });
      index += 1;
    }
  }
  if (contexts.length === 0) {
    contexts.push({ index: 0, block: null, values: {} });
  }
  return contexts;
}

/** True when the spec represents a Scenario Outline (has example rows). */
export function isOutline(spec) {
  return (spec?.examples ?? []).some((block) => (block.rows ?? []).length > 0);
}
