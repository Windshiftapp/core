import { describe, expect, it } from 'vitest';
import { isSafeUrl } from './milkdown-link-sanitizer.js';

describe('isSafeUrl', () => {
  describe('safe URLs', () => {
    it('allows https URLs', () => {
      expect(isSafeUrl('https://example.com')).toBe(true);
    });

    it('allows http URLs', () => {
      expect(isSafeUrl('http://example.com')).toBe(true);
    });

    it('allows mailto URLs', () => {
      expect(isSafeUrl('mailto:test@example.com')).toBe(true);
    });

    it('allows tel URLs', () => {
      expect(isSafeUrl('tel:+1234567890')).toBe(true);
    });

    it('allows fragment-only URLs', () => {
      expect(isSafeUrl('#section')).toBe(true);
    });

    it('allows relative URLs', () => {
      expect(isSafeUrl('/about')).toBe(true);
    });

    it('allows relative paths without leading slash', () => {
      expect(isSafeUrl('about/page')).toBe(true);
    });

    it('allows empty strings', () => {
      expect(isSafeUrl('')).toBe(true);
    });

    it('allows null/undefined', () => {
      expect(isSafeUrl(null)).toBe(true);
      expect(isSafeUrl(undefined)).toBe(true);
    });
  });

  describe('dangerous URLs', () => {
    it('blocks javascript: URLs', () => {
      expect(isSafeUrl('javascript:alert(1)')).toBe(false);
    });

    it('blocks javascript: case-insensitive', () => {
      expect(isSafeUrl('JaVaScRiPt:alert(1)')).toBe(false);
    });

    it('blocks javascript: with leading spaces', () => {
      expect(isSafeUrl('  javascript:alert(1)')).toBe(false);
    });

    it('blocks vbscript: URLs', () => {
      expect(isSafeUrl('vbscript:MsgBox("xss")')).toBe(false);
    });

    it('blocks data: URLs', () => {
      expect(isSafeUrl('data:text/html,<script>alert(1)</script>')).toBe(false);
    });

    it('blocks data: image URLs (SVG XSS)', () => {
      expect(isSafeUrl('data:image/svg+xml,<svg onload=alert(1)>')).toBe(false);
    });
  });
});
