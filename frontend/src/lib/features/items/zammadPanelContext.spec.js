import { describe, expect, it } from 'vitest';
import {
  isCurrentZammadMetadataRequest,
  isCurrentZammadPanelContext,
  isUsableZammadGroup,
} from './zammadPanelContext.js';

describe('Zammad item panel context guards', () => {
  it('rejects a response from the previous item after a prop change', () => {
    expect(
      isCurrentZammadPanelContext(1, 2, 'old-item', 'new-item', 'workspace', 'workspace')
    ).toBe(false);
  });

  it('rejects a response from the previous workspace even when the item id is reused', () => {
    expect(
      isCurrentZammadPanelContext(3, 3, 'item', 'item', 'old-workspace', 'new-workspace')
    ).toBe(false);
  });

  it('accepts a response for the current item and workspace', () => {
    expect(isCurrentZammadPanelContext(4, 4, 'item', 'item', 'workspace', 'workspace')).toBe(true);
  });
});

describe('Zammad metadata request guards', () => {
  it('rejects create metadata after switching to the link dialog', () => {
    expect(isCurrentZammadMetadataRequest(1, 1, 'connection', 'connection', true, 'link')).toBe(
      false
    );
  });

  it('rejects create metadata after the dialog closes', () => {
    expect(isCurrentZammadMetadataRequest(2, 2, 'connection', 'connection', false, 'create')).toBe(
      false
    );
  });

  it('accepts metadata only for the active create dialog and connection', () => {
    expect(isCurrentZammadMetadataRequest(3, 3, 'connection', 'connection', true, 'create')).toBe(
      true
    );
  });
});

describe('Zammad group choices', () => {
  it('offers only active groups with a verified persisted name', () => {
    expect(isUsableZammadGroup({ id: 2, name: 'Windshift', active: true })).toBe(true);
    expect(isUsableZammadGroup({ id: 3, name: '   ', active: true })).toBe(false);
    expect(isUsableZammadGroup({ id: 4, name: 'Legacy', active: false })).toBe(false);
  });
});
