import { describe, expect, it } from 'vitest';
import {
  decodePageDiagramPayload,
  pageDiagramSceneFingerprint,
  preparePageDiagramScene,
} from './pageDiagramScene.js';

const scene = {
  elements: [
    {
      id: 'rectangle-1',
      type: 'rectangle',
      x: 10,
      y: 20,
      width: 120,
      height: 80,
      strokeColor: '#000000',
    },
  ],
  appState: { viewBackgroundColor: '#ffffff' },
  files: {},
};

describe('Page diagram scene payloads', () => {
  it('decodes a stored Excalidraw scene and supplies editor defaults', () => {
    expect(decodePageDiagramPayload(JSON.stringify(scene))).toEqual({
      kind: 'excalidraw',
      scene: { ...scene, scrollToContent: true },
    });
  });

  it('decodes a Mermaid seed without converting it prematurely', () => {
    expect(decodePageDiagramPayload({ type: 'mermaid', source: 'graph TD\nA-->B' })).toEqual({
      kind: 'mermaid',
      source: 'graph TD\nA-->B',
    });
  });

  it('converts a Mermaid seed once when preparing editor data', async () => {
    const parseMermaid = async (source) => {
      expect(source).toBe('graph TD\nA-->B');
      return { elements: [{ id: 'seed', type: 'rectangle' }], files: {} };
    };
    const convertElements = (elements) => elements.map((element) => ({ ...element, converted: true }));

    await expect(
      preparePageDiagramScene(
        { type: 'mermaid', source: 'graph TD\nA-->B' },
        { parseMermaid, convertElements }
      )
    ).resolves.toEqual({
      elements: [{ id: 'seed', type: 'rectangle', converted: true }],
      appState: {},
      files: {},
      scrollToContent: true,
    });
  });

  it('rejects malformed scenes before they reach the editor', () => {
    expect(() => decodePageDiagramPayload({ elements: 'not-an-array' })).toThrow(
      'Excalidraw scene elements are missing'
    );
    expect(() => decodePageDiagramPayload({ type: 'mermaid', source: ' ' })).toThrow(
      'Mermaid diagram source is missing'
    );
  });

  it('detects element content, position, style, and file changes', () => {
    const baseline = pageDiagramSceneFingerprint(scene);

    expect(
      pageDiagramSceneFingerprint({
        ...scene,
        elements: [{ ...scene.elements[0], text: 'Changed' }],
      })
    ).not.toBe(baseline);
    expect(
      pageDiagramSceneFingerprint({
        ...scene,
        elements: [{ ...scene.elements[0], x: 11 }],
      })
    ).not.toBe(baseline);
    expect(
      pageDiagramSceneFingerprint({
        ...scene,
        elements: [{ ...scene.elements[0], strokeColor: '#ff0000' }],
      })
    ).not.toBe(baseline);
    expect(
      pageDiagramSceneFingerprint({ ...scene, files: { image: { data: 'changed' } } })
    ).not.toBe(baseline);
  });
});
