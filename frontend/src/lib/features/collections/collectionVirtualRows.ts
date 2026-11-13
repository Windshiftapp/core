export function collectionItemRowKey(item: {
  id: number | string;
  workspace_id?: number | string;
}) {
  return `item:${item.workspace_id ?? 'global'}:${item.id}`;
}

export function collectionTreeRowKey(item: {
  id: number | string;
  workspace_id?: number | string;
  isTestCase?: boolean;
  ownerItemId?: number;
  isLoadMore?: boolean;
  loadMoreFor?: number;
  testCaseData?: { workspace_id?: number };
}) {
  if (item.isLoadMore) return `more:${item.loadMoreFor}`;
  if (item.isTestCase)
    return `test:${item.ownerItemId}:${item.testCaseData?.workspace_id ?? item.workspace_id}:${item.id}`;
  return collectionItemRowKey(item);
}
