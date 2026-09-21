import DOMPurify from 'dompurify';
import { marked } from 'marked';

marked.setOptions({
  gfm: true,
  breaks: false,
  // pedantic / mangle / headerIds aren't options on this marked version; rely on defaults.
});

/**
 * Render a Markdown string to sanitized HTML.
 *
 * Only for static, non-editor contexts such as OpenAPI `description` fields
 * in the API docs. App markdown views render through MilkdownEditor in
 * readonly mode so editing and viewing stay visually identical.
 */
export function renderMarkdown(input) {
  if (!input) return '';
  const raw = marked.parse(String(input), { async: false });
  return DOMPurify.sanitize(raw, { USE_PROFILES: { html: true } });
}
