export function isCurrentZammadPanelContext(
  requestVersion,
  currentVersion,
  requestItemId,
  itemId,
  requestWorkspaceId,
  workspaceId
) {
  return (
    requestVersion === currentVersion &&
    requestItemId === itemId &&
    requestWorkspaceId === workspaceId
  );
}

export function isCurrentZammadMetadataRequest(
  requestVersion,
  currentVersion,
  requestConnectionId,
  selectedConnectionId,
  showCreate,
  dialogMode
) {
  return (
    requestVersion === currentVersion &&
    requestConnectionId === selectedConnectionId &&
    showCreate &&
    dialogMode === 'create'
  );
}

export function isUsableZammadGroup(group) {
  return group?.active !== false && Boolean(group?.name?.trim());
}
