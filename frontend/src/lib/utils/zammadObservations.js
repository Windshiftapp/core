function nonEmptyName(value) {
  return typeof value?.name === 'string' ? value.name.trim() : '';
}

function positiveId(value) {
  const id = Number(value?.id);
  return Number.isFinite(id) && id > 0 ? value.id : null;
}

export function getZammadObservedValueLabel(value, translate, field = null) {
  if (field === 'owner' && Number(value?.id) === 1) {
    return translate('zammad.unassignedOwner');
  }

  const name = nonEmptyName(value);
  if (name) return name;

  const id = positiveId(value);
  if (id !== null) return translate('zammad.timeline.valueIdFallback', { id });

  return translate('zammad.timeline.initialValue');
}

export function getZammadStatusBucketLabel(status, translate) {
  const name = nonEmptyName(status);
  if (name) return name;

  const id = positiveId(status);
  if (id !== null) return translate('zammad.overview.statusIdFallback', { id });

  return translate('zammad.overview.unknownStatusBucket');
}

export function getZammadStatusBucketDisplayLabel(status, translate) {
  const statusLabel = getZammadStatusBucketLabel(status, translate);
  const connectionName =
    typeof status?.connection_name === 'string' ? status.connection_name.trim() : '';
  if (!connectionName) return statusLabel;

  return translate('zammad.overview.statusWithConnection', {
    connection: connectionName,
    status: statusLabel,
  });
}

export function getZammadStatusAppearance(status, closed = status?.closed === true) {
  if (closed) return 'success';
  if (positiveId(status) === null) return 'default';

  const normalizedName = nonEmptyName(status).toLowerCase();
  if (/^pending(?:[\s_-]|$)/.test(normalizedName)) return 'warning';

  return 'info';
}

export function isCurrentZammadWorkspaceOverviewRequest(
  requestVersion,
  currentVersion,
  requestWorkspaceId,
  workspaceId
) {
  return requestVersion === currentVersion && String(requestWorkspaceId) === String(workspaceId);
}
