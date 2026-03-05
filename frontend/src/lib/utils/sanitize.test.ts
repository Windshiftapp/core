import { describe, expect, it } from 'vitest';
import { escapeHtml, sanitizeHtml, stripHtml } from './sanitize';

describe('sanitizeHtml', () => {
  it('strips script tags', () => {
    expect(sanitizeHtml('<script>alert(1)</script>')).toBe('');
  });

  it('strips event handlers', () => {
    expect(sanitizeHtml('<img src="x" onerror="alert(1)">')).toBe('<img src="x">');
  });

  it('removes javascript: hrefs', () => {
    expect(sanitizeHtml('<a href="javascript:alert(1)">click</a>')).toBe('<a>click</a>');
  });

  it('removes data: URIs on anchors', () => {
    expect(sanitizeHtml('<a href="data:text/html,<script>alert(1)</script>">click</a>')).toBe(
      '<a>click</a>'
    );
  });

  it('preserves safe formatting tags', () => {
    const input = '<strong>bold</strong> <em>italic</em> <a href="https://example.com">link</a>';
    expect(sanitizeHtml(input)).toBe(
      '<strong>bold</strong> <em>italic</em> <a href="https://example.com">link</a>'
    );
  });

  it('preserves list elements', () => {
    const input = '<ul><li>item 1</li><li>item 2</li></ul>';
    expect(sanitizeHtml(input)).toBe(input);
  });

  it('preserves code blocks', () => {
    const input = '<pre><code>const x = 1;</code></pre>';
    expect(sanitizeHtml(input)).toBe(input);
  });

  it('preserves img tags with safe attributes', () => {
    expect(sanitizeHtml('<img src="photo.jpg" alt="photo">')).toBe(
      '<img src="photo.jpg" alt="photo">'
    );
  });

  it('strips onclick from div', () => {
    expect(sanitizeHtml('<div onclick="alert(1)">text</div>')).toBe('<div>text</div>');
  });

  it('strips iframe tags', () => {
    expect(sanitizeHtml('<iframe src="evil.com"></iframe>')).toBe('');
  });

  it('handles null/undefined/empty', () => {
    expect(sanitizeHtml('')).toBe('');
    expect(sanitizeHtml(null as unknown as string)).toBe('');
    expect(sanitizeHtml(undefined as unknown as string)).toBe('');
  });

  it('preserves SVG elements', () => {
    const input =
      '<svg viewBox="0 0 24 24" width="16" height="16"><circle cx="12" cy="12" r="10" fill="green"></circle></svg>';
    expect(sanitizeHtml(input)).toBe(input);
  });
});

describe('stripHtml', () => {
  it('strips all HTML tags', () => {
    expect(stripHtml('<b>Bold</b> and <i>italic</i>')).toBe('Bold and italic');
  });

  it('strips script tags and content', () => {
    expect(stripHtml('<script>alert(1)</script>text')).toBe('text');
  });

  it('strips event handlers', () => {
    expect(stripHtml('<img src="x" onerror="alert(1)">')).toBe('');
  });

  it('handles null/undefined/empty', () => {
    expect(stripHtml('')).toBe('');
    expect(stripHtml(null as unknown as string)).toBe('');
    expect(stripHtml(undefined as unknown as string)).toBe('');
  });
});

describe('escapeHtml', () => {
  it('escapes HTML entities', () => {
    expect(escapeHtml('<script>alert("xss")</script>')).toBe(
      '&lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;'
    );
  });

  it('escapes ampersands', () => {
    expect(escapeHtml('foo & bar')).toBe('foo &amp; bar');
  });

  it('escapes single quotes', () => {
    expect(escapeHtml("it's")).toBe('it&#39;s');
  });

  it('handles null/undefined', () => {
    expect(escapeHtml(null)).toBe('');
    expect(escapeHtml(undefined)).toBe('');
  });

  it('handles numbers', () => {
    expect(escapeHtml(42)).toBe('42');
  });

  it('handles plain text unchanged', () => {
    expect(escapeHtml('hello world')).toBe('hello world');
  });
});
