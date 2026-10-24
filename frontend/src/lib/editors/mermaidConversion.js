// Shared Mermaid -> Excalidraw conversion used by Page diagrams and the
// work-item diagram editor. The upstream converter queries subgraph clusters
// by their unprefixed id, but mermaid >= 11.14 prefixes every rendered element
// id with the render id. Strip that prefix from the SVG string the converter
// parses so subgraph lookups resolve; without it the converter throws
// "SubGraph element not found" and silently degrades to a static image.

const CONVERTER_RENDER_ID_PREFIX = 'mermaid-to-excalidraw-';

let mermaidModulePromise = null;
function loadMermaid() {
  if (!mermaidModulePromise) {
    mermaidModulePromise = import('mermaid').then((mod) => mod.default ?? mod);
  }
  return mermaidModulePromise;
}

// Install the render-id fix once. It only rewrites SVGs produced for the
// converter's own render ids, so direct Mermaid renders are untouched.
let renderIdFixPromise = null;
function ensureRenderIdFix() {
  if (!renderIdFixPromise) {
    renderIdFixPromise = loadMermaid().then((mermaid) => {
      const baseRender = mermaid.render;
      mermaid.render = async function renderWithoutIdPrefix(id, text, container) {
        const result = await baseRender.call(this, id, text, container);
        if (
          typeof id === 'string' &&
          id.startsWith(CONVERTER_RENDER_ID_PREFIX) &&
          result &&
          typeof result.svg === 'string'
        ) {
          result.svg = stripRenderIdPrefix(result.svg, id);
        }
        return result;
      };
    });
  }
  return renderIdFixPromise;
}

function stripRenderIdPrefix(svg, renderId) {
  const escaped = renderId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return svg.replace(new RegExp(`(id=")${escaped}-`, 'g'), '$1');
}

// Mermaid renders `<br/>` labels as line breaks, but the converter copies the
// raw tag into Excalidraw text. Normalize it to real newlines so labels wrap.
function normalizeBreakTags(elements) {
  const fix = (value) => (typeof value === 'string' ? value.replace(/<br\s*\/?>/gi, '\n') : value);
  return elements.map((element) => {
    const next = { ...element };
    if (next.label && typeof next.label.text === 'string') {
      next.label = { ...next.label, text: fix(next.label.text) };
    }
    if (typeof next.text === 'string') {
      next.text = fix(next.text);
    }
    return next;
  });
}

export async function parseMermaidToExcalidrawElements(source) {
  const { parseMermaidToExcalidraw } = await import('@excalidraw/mermaid-to-excalidraw');
  await ensureRenderIdFix();
  const { elements, files } = await parseMermaidToExcalidraw(source);
  return { elements: normalizeBreakTags(elements), files: files || {} };
}

export async function convertMermaidToExcalidraw(source) {
  const { elements, files } = await parseMermaidToExcalidrawElements(source);
  const { convertToExcalidrawElements } = await import('@excalidraw/excalidraw');
  return { elements: convertToExcalidrawElements(elements), files };
}
