import { pageToPagePeerDirection } from './requirementLinkLabels.js';

/**
 * @param {{ outgoing?: unknown[], incoming?: unknown[] } | null | undefined} resp
 * @returns {Record<string, unknown>[]}
 */
export function mergePageLinks(resp) {
  const outgoing = Array.isArray(resp?.outgoing) ? resp.outgoing : [];
  const incoming = Array.isArray(resp?.incoming) ? resp.incoming : [];
  const seen = new Set();
  const merged = [];
  for (const link of [...incoming, ...outgoing]) {
    if (link && link.id != null && !seen.has(link.id)) {
      seen.add(link.id);
      merged.push(link);
    }
  }
  return merged;
}

/**
 * @param {Record<string, unknown>[]} linkTypes
 */
export function resolveLinkTypeIds(linkTypes) {
  const findId = (predicate) => linkTypes.find(predicate)?.id ?? null;
  const pageLinkTypeId = findId((lt) => lt?.builtin_key === 'page' || lt?.name === 'Page');
  const testsLinkTypeId = findId((lt) => lt?.builtin_key === 'tests' || lt?.name === 'Tests');
  const relatesToLinkTypeId = findId(
    (lt) => lt?.builtin_key === 'relates_to' || lt?.name === 'Relates To'
  );
  const implementsLinkTypeId = findId((lt) => lt?.builtin_key === 'implements');
  const specifiesLinkTypeId = findId((lt) => lt?.builtin_key === 'specifies');
  const itemLinkTypeId = implementsLinkTypeId ?? pageLinkTypeId;
  return {
    pageLinkTypeId,
    testsLinkTypeId,
    relatesToLinkTypeId,
    implementsLinkTypeId,
    specifiesLinkTypeId,
    itemLinkTypeId,
  };
}

/**
 * @param {Record<string, unknown>} link
 * @param {number} pageId
 */
export function linkedEntity(link, pageId) {
  const pageIsSource = link.source_type === 'page' && link.source_id === pageId;
  const pageIsTarget = link.target_type === 'page' && link.target_id === pageId;
  if (pageIsSource) {
    return {
      type: link.target_type,
      id: link.target_id,
      title: link.target_title,
      workspaceId: link.target_workspace_id,
      workspaceKey: link.target_workspace_key,
      itemNumber: link.target_item_number,
      statusName: link.target_status_name,
      statusColor: link.target_status_color,
      itemTypeIcon: link.target_item_type_icon,
      itemTypeColor: link.target_item_type_color,
    };
  }
  if (pageIsTarget) {
    return {
      type: link.source_type,
      id: link.source_id,
      title: link.source_title,
      workspaceId: link.source_workspace_id,
      workspaceKey: link.source_workspace_key,
      itemNumber: link.source_item_number,
      statusName: link.source_status_name,
      statusColor: link.source_status_color,
      itemTypeIcon: link.source_item_type_icon,
      itemTypeColor: link.source_item_type_color,
    };
  }
  return null;
}

/**
 * @param {Record<string, unknown>[]} pageLinks
 * @param {number} pageId
 */
function peerEntityType(link, pageId) {
  if (link.source_type === 'page' && link.source_id === pageId) return link.target_type;
  if (link.target_type === 'page' && link.target_id === pageId) return link.source_type;
  return null;
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId */
export function filterItemLinks(pageLinks, pageId) {
  return pageLinks.filter((link) => peerEntityType(link, pageId) === 'item');
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId */
export function filterTestLinks(pageLinks, pageId) {
  return pageLinks.filter((link) => peerEntityType(link, pageId) === 'test_case');
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId @param {number | null} specifiesLinkTypeId */
export function filterSpecifiesOutgoing(pageLinks, pageId, specifiesLinkTypeId) {
  return pageLinks.filter((link) => {
    if (link.link_type_id !== specifiesLinkTypeId) return false;
    return pageToPagePeerDirection(link, pageId) === 'outgoing';
  });
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId @param {number | null} specifiesLinkTypeId */
export function filterSpecifiedBy(pageLinks, pageId, specifiesLinkTypeId) {
  return pageLinks.filter((link) => {
    if (link.link_type_id !== specifiesLinkTypeId) return false;
    return pageToPagePeerDirection(link, pageId) === 'incoming';
  });
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId @param {number | null} relatesToLinkTypeId */
export function filterGenericPageLinks(pageLinks, pageId, relatesToLinkTypeId) {
  return pageLinks.filter((link) => {
    if (link.link_type_id !== relatesToLinkTypeId) return false;
    return pageToPagePeerDirection(link, pageId) != null;
  });
}

/** @param {Record<string, unknown>[]} pageLinks @param {number} pageId */
export function filterAssetLinks(pageLinks, pageId) {
  return pageLinks.filter((link) => peerEntityType(link, pageId) === 'asset');
}

/** @param {number} pageId @param {{ id: number }} item @param {number} itemLinkTypeId */
export function buildItemLinkCreate(pageId, item, itemLinkTypeId) {
  return {
    link_type_id: itemLinkTypeId,
    source_type: 'item',
    source_id: item.id,
    target_type: 'page',
    target_id: pageId,
  };
}

/** @param {number} pageId @param {{ id: number }} testCase @param {number} testsLinkTypeId */
export function buildTestLinkCreate(pageId, testCase, testsLinkTypeId) {
  return {
    link_type_id: testsLinkTypeId,
    source_type: 'page',
    source_id: pageId,
    target_type: 'test_case',
    target_id: testCase.id,
  };
}

/** @param {number} pageId @param {{ id: number }} relatedPage @param {number} specifiesLinkTypeId */
export function buildSpecifiesLinkCreate(pageId, relatedPage, specifiesLinkTypeId) {
  return {
    link_type_id: specifiesLinkTypeId,
    source_type: 'page',
    source_id: pageId,
    target_type: 'page',
    target_id: relatedPage.id,
  };
}

/** @param {number} pageId @param {{ id: number }} relatedPage @param {number} relatesToLinkTypeId */
export function buildGenericPageLinkCreate(pageId, relatedPage, relatesToLinkTypeId) {
  return {
    link_type_id: relatesToLinkTypeId,
    source_type: 'page',
    source_id: pageId,
    target_type: 'page',
    target_id: relatedPage.id,
  };
}

/** @param {number} pageId @param {{ id: number }} asset @param {number} relatesToLinkTypeId */
export function buildAssetLinkCreate(pageId, asset, relatesToLinkTypeId) {
  return {
    link_type_id: relatesToLinkTypeId,
    source_type: 'page',
    source_id: pageId,
    target_type: 'asset',
    target_id: asset.id,
  };
}
