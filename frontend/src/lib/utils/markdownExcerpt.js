// Plain-text excerpt of a Markdown string, for places that cannot render
// Markdown (e.g. a `title` tooltip). Strips common Markdown syntax, keeps the
// first paragraph that has text, collapses whitespace, and truncates to about
// `maxLength` characters with an ellipsis. Returns '' when nothing is left.

export const DEFAULT_EXCERPT_LENGTH = 150;

// Private-use placeholders keep backslash-escaped characters (\*, \_, ...)
// out of the emphasis/link rules; they are restored at the end.
const ESCAPE_START = '';
const ESCAPE_END = '';

const NAMED_ENTITIES = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: ' ',
};

function decodeEntities(text) {
  return text.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (match, entity) => {
    if (entity[0] === '#') {
      const code =
        entity[1] === 'x' || entity[1] === 'X'
          ? parseInt(entity.slice(2), 16)
          : parseInt(entity.slice(1), 10);
      return Number.isFinite(code) && code > 0 && code <= 0x10ffff
        ? String.fromCodePoint(code)
        : match;
    }
    return NAMED_ENTITIES[entity.toLowerCase()] ?? match;
  });
}

// Fenced code blocks make poor excerpts: drop them, leaving a blank line so
// the text around them stays in separate paragraphs. An unclosed fence runs
// to the end of the input, as in CommonMark.
function dropFencedCode(text) {
  const out = [];
  let fence = null;
  for (const line of text.split('\n')) {
    const marker = /^\s{0,3}(`{3,}|~{3,})/.exec(line)?.[1];
    if (fence) {
      if (marker && marker[0] === fence[0] && marker.length >= fence.length) fence = null;
      continue;
    }
    if (marker) {
      fence = marker;
      out.push('');
      continue;
    }
    out.push(line);
  }
  return out.join('\n');
}

function stripBlockSyntax(line) {
  // Horizontal rules and setext heading underlines carry no text.
  if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line) || /^\s*=+\s*$/.test(line)) return '';
  // Table delimiter rows (| --- | :---: |).
  if (/^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)+\|?\s*$/.test(line)) return '';
  // Reference-style link definitions ([ref]: https://...).
  if (/^\s{0,3}\[[^\]]+\]:\s*\S+/.test(line)) return '';

  let result = line;
  result = result.replace(/^\s*(>\s?)+/, ''); // blockquotes
  result = result.replace(/^\s{0,3}#{1,6}\s+/, '').replace(/\s+#+\s*$/, ''); // ATX headings
  result = result.replace(/^\s*([-*+]|\d+[.)])\s+/, ''); // list markers
  result = result.replace(/^\[[ xX]\]\s+/, ''); // task list boxes
  return result;
}

function stripInlineSyntax(text) {
  let result = text;
  result = result.replace(/!\[[^\]]*\]\([^)]*\)/g, ''); // images
  result = result.replace(/!\[[^\]]*\]\[[^\]]*\]/g, ''); // reference images
  result = result.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1'); // inline links
  result = result.replace(/\[([^\]]+)\]\[[^\]]*\]/g, '$1'); // reference links
  result = result.replace(/<((?:https?|mailto):[^>\s]+)>/gi, '$1'); // autolinks
  result = result.replace(/<[^>]+>/g, ' '); // HTML tags
  result = result.replace(/(`+)([^`]*?)\1/g, '$2'); // inline code
  result = result.replace(/(\*\*|__)(?=\S)([\s\S]*?\S)\1/g, '$2'); // bold
  result = result.replace(/~~(?=\S)([\s\S]*?\S)~~/g, '$1'); // strikethrough
  result = result.replace(/\*(?=\S)([^*]*?\S)\*/g, '$1'); // italic (*)
  result = result.replace(/(^|[^\w])_(?=\S)([^_]*?\S)_(?=[^\w]|$)/g, '$1$2'); // italic (_)
  return result;
}

function truncate(text, maxLength) {
  if (text.length <= maxLength) return text;
  let cut = text.slice(0, maxLength);
  const lastSpace = cut.lastIndexOf(' ');
  // Prefer a word boundary unless that would throw away too much text.
  if (lastSpace > maxLength * 0.6) cut = cut.slice(0, lastSpace);
  return `${cut.replace(/[\s.,;:!?-]+$/, '')}…`;
}

/**
 * @param {string | null | undefined} markdown
 * @param {number} [maxLength]
 * @returns {string}
 */
export function markdownExcerpt(markdown, maxLength = DEFAULT_EXCERPT_LENGTH) {
  if (typeof markdown !== 'string' || markdown.trim() === '') return '';

  let text = markdown.replace(/\r\n?/g, '\n');
  text = text.replace(
    /\\([\\`*_{}[\]()#+\-.!>~|<])/g,
    (_, char) => `${ESCAPE_START}${char.codePointAt(0)}${ESCAPE_END}`
  );
  text = dropFencedCode(text);
  text = text.replace(/<!--[\s\S]*?-->/g, ''); // HTML comments

  const paragraphs = text.split(/\n\s*\n/);
  for (const paragraph of paragraphs) {
    const joined = paragraph.split('\n').map(stripBlockSyntax).join(' ');
    let plain = stripInlineSyntax(joined);
    plain = plain.replace(new RegExp(`${ESCAPE_START}(\\d+)${ESCAPE_END}`, 'g'), (_, code) =>
      String.fromCodePoint(Number(code))
    );
    plain = decodeEntities(plain).replace(/\s+/g, ' ').trim();
    if (plain) return truncate(plain, maxLength);
  }
  return '';
}
