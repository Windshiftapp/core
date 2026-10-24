// Suppress a program title that only repeats the tile heading.
//
// The dashboard prints the widget's name above every tile, so a program that
// starts with `Title("<the same name>")` renders the words twice. The prompt
// tells the model not to do that, but widgets saved before it did still carry
// the duplicate, and a hand edit can add one back.
//
// The renderer only accepts source text, so this rewrites the program instead
// of the parsed tree: the leading Title call is removed from the root's
// children, along with the statement that defined it.

/** Normalize a title for comparison: case, whitespace, and trailing punctuation. */
export function normalizeTitle(value) {
  return String(value ?? '')
    .trim()
    .toLowerCase()
    .replace(/[\s.!?:;,–—-]+$/u, '')
    .replace(/\s+/gu, ' ');
}

const STATEMENT_RE = /^[ \t]*([A-Za-z_][\w]*)[ \t]*=/gm;

/** Split a program into statements with their source spans. */
function statements(program) {
  const starts = [...program.matchAll(STATEMENT_RE)];
  return starts.map((match, index) => {
    const from = match.index;
    const to = index + 1 < starts.length ? starts[index + 1].index : program.length;
    return { id: match[1], text: program.slice(from, to), from, to };
  });
}

/**
 * Remove the leading Title when it repeats `tileTitle`. Returns `program`
 * unchanged when there is nothing to do.
 *
 * @param {string} program OpenUI Lang source
 * @param {string} tileTitle the heading the dashboard already renders
 * @returns {string}
 */
export function stripDuplicateTitle(program, tileTitle) {
  const wanted = normalizeTitle(tileTitle);
  if (!program || !wanted) return program;

  const parts = statements(program);
  const root = parts.find((part) => part.id === 'root');
  if (!root) return program;

  // The duplicate must be the root's first child: either an inline Title call
  // with a matching literal, or a reference to a Title statement.
  const inline = /\[\s*Title\s*\(\s*"((?:[^"\\]|\\.)*)"/.exec(root.text);
  const referenced = /\[\s*([A-Za-z_][\w]*)\s*,/.exec(root.text);

  const matchesInline = inline && normalizeTitle(inline[1].replace(/\\"/g, '"')) === wanted;
  const refId = referenced?.[1];
  const refStatement = refId ? parts.find((part) => part.id === refId) : null;
  const matchesReference =
    refStatement &&
    new RegExp(`^\\s*${refId}\\s*=\\s*Title\\s*\\(\\s*"((?:[^"\\\\]|\\\\.)*)"`).exec(
      refStatement.text
    );
  const referenceTitle =
    matchesReference && normalizeTitle(matchesReference[1].replace(/\\"/g, '"')) === wanted;

  if (!matchesInline && !referenceTitle) return program;

  // Drop the Title statement when the root referenced it.
  if (referenceTitle) {
    program = program.slice(0, refStatement.from) + program.slice(refStatement.to);
  }

  // Recompute the root span after any statement removal, then drop the child.
  const refreshedRoot = statements(program).find((part) => part.id === 'root');
  if (!refreshedRoot) return program;
  const childName = referenceTitle ? refId : 'Title';
  const withoutChild = refreshedRoot.text.replace(
    new RegExp(`(\\[\\s*)${childName}(\\s*\\([^)]*\\))?\\s*,\\s*`),
    '$1'
  );
  if (withoutChild === refreshedRoot.text) return program;

  return program.slice(0, refreshedRoot.from) + withoutChild + program.slice(refreshedRoot.to);
}
