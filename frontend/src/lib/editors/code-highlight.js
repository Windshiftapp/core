import { createHighlighterCore } from 'shiki/core';
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript';

const LANG_LOADERS = {
  js: () => import('shiki/langs/javascript.mjs'),
  javascript: () => import('shiki/langs/javascript.mjs'),
  ts: () => import('shiki/langs/typescript.mjs'),
  typescript: () => import('shiki/langs/typescript.mjs'),
  jsx: () => import('shiki/langs/jsx.mjs'),
  tsx: () => import('shiki/langs/tsx.mjs'),
  go: () => import('shiki/langs/go.mjs'),
  python: () => import('shiki/langs/python.mjs'),
  py: () => import('shiki/langs/python.mjs'),
  bash: () => import('shiki/langs/bash.mjs'),
  sh: () => import('shiki/langs/bash.mjs'),
  shell: () => import('shiki/langs/shellscript.mjs'),
  shellscript: () => import('shiki/langs/shellscript.mjs'),
  json: () => import('shiki/langs/json.mjs'),
  yaml: () => import('shiki/langs/yaml.mjs'),
  yml: () => import('shiki/langs/yaml.mjs'),
  sql: () => import('shiki/langs/sql.mjs'),
  html: () => import('shiki/langs/html.mjs'),
  css: () => import('shiki/langs/css.mjs'),
  scss: () => import('shiki/langs/scss.mjs'),
  svelte: () => import('shiki/langs/svelte.mjs'),
  markdown: () => import('shiki/langs/markdown.mjs'),
  md: () => import('shiki/langs/markdown.mjs'),
  diff: () => import('shiki/langs/diff.mjs'),
  rust: () => import('shiki/langs/rust.mjs'),
  rs: () => import('shiki/langs/rust.mjs'),
  java: () => import('shiki/langs/java.mjs'),
  c: () => import('shiki/langs/c.mjs'),
  cpp: () => import('shiki/langs/cpp.mjs'),
  ruby: () => import('shiki/langs/ruby.mjs'),
  rb: () => import('shiki/langs/ruby.mjs'),
  php: () => import('shiki/langs/php.mjs'),
  xml: () => import('shiki/langs/xml.mjs'),
  toml: () => import('shiki/langs/toml.mjs'),
};

const THEMES = { light: 'github-light', dark: 'github-dark' };

let highlighterPromise = null;
const loadedLangs = new Set();
let loadingLangs = new Map();

function getHighlighter() {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighterCore({
      themes: [import('shiki/themes/github-light.mjs'), import('shiki/themes/github-dark.mjs')],
      langs: [],
      engine: createJavaScriptRegexEngine(),
    });
  }
  return highlighterPromise;
}

async function ensureLang(lang) {
  if (!lang || !LANG_LOADERS[lang]) return null;
  if (loadedLangs.has(lang)) return lang;
  if (loadingLangs.has(lang)) {
    await loadingLangs.get(lang);
    return lang;
  }
  const hl = await getHighlighter();
  const p = (async () => {
    const mod = await LANG_LOADERS[lang]();
    await hl.loadLanguage(mod.default ?? mod);
    loadedLangs.add(lang);
  })();
  loadingLangs.set(lang, p);
  try {
    await p;
  } finally {
    loadingLangs.delete(lang);
  }
  return lang;
}

function detectLang(codeEl) {
  const cls = codeEl.className || '';
  const match = cls.match(/language-([\w-]+)/i);
  if (!match) return null;
  return match[1].toLowerCase();
}

export async function highlightCodeBlocks(root) {
  if (!root) return;
  const nodes = root.querySelectorAll('pre > code');
  if (nodes.length === 0) return;

  const hl = await getHighlighter();

  for (const codeEl of nodes) {
    const pre = codeEl.parentElement;
    if (!pre || pre.dataset.shiki === '1') continue;

    const rawLang = detectLang(codeEl);
    const lang = rawLang ? await ensureLang(rawLang) : null;
    const src = codeEl.textContent ?? '';

    try {
      const html = hl.codeToHtml(src, {
        lang: lang || 'text',
        themes: THEMES,
        defaultColor: false,
      });
      const tpl = document.createElement('template');
      tpl.innerHTML = html.trim();
      const next = tpl.content.firstElementChild;
      if (next) {
        next.dataset.shiki = '1';
        pre.replaceWith(next);
      }
    } catch (e) {
      console.warn('shiki highlight failed', { lang, error: e });
    }
  }
}
