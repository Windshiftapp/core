import { collectionItemRowKey } from './collectionVirtualRows';

interface BacklogItem {
  id: number;
  workspace_id?: number;
}
interface BacklogSection {
  iteration: { id: number } | null;
  items: BacklogItem[];
}

export function projectBacklogRows(sections: BacklogSection[], collapsed: Set<number | string>) {
  return sections.flatMap((section) => {
    const sectionId = section.iteration?.id ?? 'unassigned';
    const header = {
      key: `section:${sectionId}`,
      kind: 'header',
      section,
      sectionId,
      item: null as BacklogItem | null,
    };
    if (collapsed.has(sectionId)) return [header];
    if (!section.items.length)
      return [header, { ...header, key: `empty:${sectionId}`, kind: 'empty' }];
    return [
      header,
      ...section.items.map((item) => ({
        key: collectionItemRowKey(item),
        kind: 'item',
        section,
        sectionId,
        item,
      })),
    ];
  });
}
