import DOMPurify from 'dompurify';

/**
 * Sanitize HTML with an allowlist of safe formatting tags.
 * Use for rendering user content that may contain HTML (e.g. markdown output, rich text).
 */
export function sanitizeHtml(dirty: string): string {
	if (!dirty) return '';
	return DOMPurify.sanitize(dirty, {
		ALLOWED_TAGS: [
			'strong', 'b', 'em', 'i', 'u', 's', 'del',
			'a', 'p', 'br', 'hr',
			'ul', 'ol', 'li',
			'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
			'code', 'pre', 'blockquote',
			'img',
			'table', 'thead', 'tbody', 'tr', 'th', 'td',
			'div', 'span',
			'svg', 'path', 'circle', 'rect', 'line', 'polyline', 'polygon',
		],
		ALLOWED_ATTR: [
			'href', 'target', 'rel', 'title', 'alt', 'src',
			'class', 'style',
			'colspan', 'rowspan',
			// SVG attributes
			'viewBox', 'width', 'height', 'fill', 'stroke', 'stroke-width',
			'stroke-linecap', 'stroke-linejoin', 'd', 'cx', 'cy', 'r',
			'x', 'y', 'x1', 'y1', 'x2', 'y2', 'points',
			'xmlns',
		],
		ALLOW_DATA_ATTR: false,
	});
}

/**
 * Strip all HTML tags, returning plain text.
 * Use when only text content is needed and no HTML should remain.
 */
export function stripHtml(dirty: string): string {
	if (!dirty) return '';
	return DOMPurify.sanitize(dirty, { ALLOWED_TAGS: [], ALLOWED_ATTR: [] });
}

/**
 * Escape HTML entities for safe interpolation in template literal HTML.
 * Use when building HTML strings with user data (e.g. DataTable render functions).
 */
export function escapeHtml(text: string | number | null | undefined): string {
	if (text == null) return '';
	const str = String(text);
	return str
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}
