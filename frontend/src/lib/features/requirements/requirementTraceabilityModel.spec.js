import { describe, expect, it } from 'vitest';
import {
  buildAssetLinkCreate,
  buildItemLinkCreate,
  buildTestLinkCreate,
  filterAssetLinks,
  filterItemLinks,
  filterTestLinks,
  linkedEntity,
  mergePageLinks,
  resolveLinkTypeIds,
} from './requirementTraceabilityModel.js';

describe('mergePageLinks', () => {
  it('deduplicates by link id and merges incoming with outgoing', () => {
    const merged = mergePageLinks({
      outgoing: [
        { id: 1, title: 'a' },
        { id: 2, title: 'b' },
      ],
      incoming: [
        { id: 2, title: 'b-dup' },
        { id: 3, title: 'c' },
      ],
    });
    expect(merged.map((l) => l.id)).toEqual([2, 3, 1]);
    expect(merged.find((l) => l.id === 2)?.title).toBe('b-dup');
  });
});

describe('resolveLinkTypeIds', () => {
  it('resolves builtin link types by key and name', () => {
    const ids = resolveLinkTypeIds([
      { id: 10, builtin_key: 'tests', name: 'Tests' },
      { id: 11, builtin_key: 'specifies' },
      { id: 12, builtin_key: 'implements' },
      { id: 13, builtin_key: 'relates_to', name: 'Relates To' },
      { id: 14, builtin_key: 'page', name: 'Page' },
    ]);
    expect(ids.testsLinkTypeId).toBe(10);
    expect(ids.specifiesLinkTypeId).toBe(11);
    expect(ids.itemLinkTypeId).toBe(12);
    expect(ids.relatesToLinkTypeId).toBe(13);
    expect(ids.pageLinkTypeId).toBe(14);
  });
});

describe('linkedEntity', () => {
  const pageId = 100;

  it('returns target entity when page is source', () => {
    const entity = linkedEntity(
      {
        source_type: 'page',
        source_id: pageId,
        target_type: 'item',
        target_id: 5,
        target_title: 'Story',
      },
      pageId
    );
    expect(entity).toEqual({ type: 'item', id: 5, title: 'Story' });
  });

  it('returns source entity when page is target', () => {
    const entity = linkedEntity(
      {
        source_type: 'test_case',
        source_id: 9,
        source_title: 'Login',
        target_type: 'page',
        target_id: pageId,
      },
      pageId
    );
    expect(entity?.type).toBe('test_case');
    expect(entity?.id).toBe(9);
    expect(entity?.title).toBe('Login');
  });
});

describe('filter helpers', () => {
  const pageId = 42;
  const links = [
    { source_type: 'page', source_id: pageId, target_type: 'item', target_id: 1 },
    { source_type: 'page', source_id: pageId, target_type: 'test_case', target_id: 2 },
    { source_type: 'page', source_id: pageId, target_type: 'asset', target_id: 3 },
  ];

  it('filters item, test, and asset links', () => {
    expect(filterItemLinks(links, pageId)).toHaveLength(1);
    expect(filterTestLinks(links, pageId)).toHaveLength(1);
    expect(filterAssetLinks(links, pageId)).toHaveLength(1);
  });
});

describe('build link create payloads', () => {
  it('builds item, test, and asset link bodies', () => {
    expect(buildItemLinkCreate(10, { id: 5 }, 1)).toEqual({
      link_type_id: 1,
      source_type: 'item',
      source_id: 5,
      target_type: 'page',
      target_id: 10,
    });
    expect(buildTestLinkCreate(10, { id: 7 }, 2)).toEqual({
      link_type_id: 2,
      source_type: 'page',
      source_id: 10,
      target_type: 'test_case',
      target_id: 7,
    });
    expect(buildAssetLinkCreate(10, { id: 8 }, 3)).toEqual({
      link_type_id: 3,
      source_type: 'page',
      source_id: 10,
      target_type: 'asset',
      target_id: 8,
    });
  });
});
