import { convertMermaidToExcalidraw } from '../../editors/mermaidConversion.js';

export class PageDiagramSceneError extends Error {
  constructor(message) {
    super(message);
    this.name = 'PageDiagramSceneError';
  }
}

export function decodePageDiagramPayload(payload) {
  let parsed = payload;
  if (typeof payload === 'string') {
    try {
      parsed = JSON.parse(payload);
    } catch {
      throw new PageDiagramSceneError('Diagram payload is not valid JSON');
    }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new PageDiagramSceneError('Diagram payload must be an object');
  }
  if (parsed.type === 'mermaid') {
    if (typeof parsed.source !== 'string' || !parsed.source.trim()) {
      throw new PageDiagramSceneError('Mermaid diagram source is missing');
    }
    return { kind: 'mermaid', source: parsed.source.trim() };
  }
  if (!Array.isArray(parsed.elements)) {
    throw new PageDiagramSceneError('Excalidraw scene elements are missing');
  }
  if (
    parsed.appState != null &&
    (typeof parsed.appState !== 'object' || Array.isArray(parsed.appState))
  ) {
    throw new PageDiagramSceneError('Excalidraw appState must be an object');
  }
  if (parsed.files != null && (typeof parsed.files !== 'object' || Array.isArray(parsed.files))) {
    throw new PageDiagramSceneError('Excalidraw files must be an object');
  }
  return {
    kind: 'excalidraw',
    scene: {
      elements: parsed.elements,
      appState: parsed.appState || {},
      files: parsed.files || {},
      scrollToContent: true,
    },
  };
}

export async function preparePageDiagramScene(payload, converters = {}) {
  const decoded = decodePageDiagramPayload(payload);
  if (decoded.kind === 'excalidraw') return decoded.scene;

  try {
    const { elements, files } = await convertMermaidScene(decoded.source, converters);
    return {
      elements,
      appState: {},
      files: files || {},
      scrollToContent: true,
    };
  } catch (error) {
    throw new PageDiagramSceneError(error?.message || 'Mermaid conversion failed');
  }
}

async function convertMermaidScene(source, converters) {
  if (converters.parseMermaid && converters.convertElements) {
    const { elements: skeletons, files } = await converters.parseMermaid(source);
    return { elements: converters.convertElements(skeletons), files };
  }
  return convertMermaidToExcalidraw(source);
}

export function pageDiagramSceneFingerprint(scene) {
  const elements = Array.isArray(scene?.elements) ? scene.elements : [];
  const files = scene?.files && typeof scene.files === 'object' ? scene.files : {};
  const viewBackgroundColor = scene?.appState?.viewBackgroundColor ?? null;
  return stableStringify({ elements, files, viewBackgroundColor });
}

function stableStringify(value) {
  if (Array.isArray(value)) {
    return `[${value.map(stableStringify).join(',')}]`;
  }
  if (value && typeof value === 'object') {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${stableStringify(value[key])}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}
