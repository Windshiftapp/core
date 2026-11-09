// The machine-translated catalogs quote every object key. Quoting an
// identifier key is never required in JavaScript, so a file that quotes some
// identifier keys and leaves others bare is a hand-edit or regeneration
// artifact. Keys that are not valid identifiers (e.g. "field-mappings") must
// stay quoted and do not signal the file's convention.
const QUOTED_IDENTIFIER_KEY_PATTERN = /^\s*"([A-Za-z_$][A-Za-z0-9_$]*)":\s*(['"`])/;
const UNQUOTED_IDENTIFIER_KEY_PATTERN = /^\s*([A-Za-z_$][A-Za-z0-9_$]*):\s*(['"`])/;

/**
 * Finds locale files that mix quoted and unquoted object keys. Files that
 * quote their keys are expected to quote all of them.
 *
 * @param {Array<{file: string, content: string}>} files
 * @returns {Array<{file: string, unquoted: Array<{line: number, key: string}>}>}
 */
export function findMixedKeyQuoting(files) {
  const problems = [];

  for (const { file, content } of files) {
    const unquoted = [];
    let quoted = 0;

    content.split('\n').forEach((line, index) => {
      if (QUOTED_IDENTIFIER_KEY_PATTERN.test(line)) {
        quoted += 1;
        return;
      }
      const match = line.match(UNQUOTED_IDENTIFIER_KEY_PATTERN);
      if (match) {
        unquoted.push({ line: index + 1, key: match[1] });
      }
    });

    if (quoted > 0 && unquoted.length > 0) {
      problems.push({ file, unquoted });
    }
  }

  return problems;
}
