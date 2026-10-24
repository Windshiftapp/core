<script>
  import DataTable from '../../components/DataTable.svelte';
  import ListCellRenderer from './ListCellRenderer.svelte';
  import {
    getListColumnLabel,
    getListColumnTableWidth,
  } from '../../utils/workItemListColumns.js';

  let {
    data = [],
    columns = [],
    workspaces = [],
    customFieldDefinitions = [],
    itemTypes = [],
    statuses = [],
    statusCategories = [],
    priorities = [],
    milestones = [],
    iterations = [],
    projects = [],
    includeWorkspace = false,
    canEdit = false,
    actionItems = null,
    emptyMessage = '',
    emptyDescription = '',
    emptyIcon = null,
    testId = 'work-item-list-table',
    onRowClick = null,
    rowAttrs = null,
  } = $props();

  function getWorkspace(item) {
    return (
      workspaces.find((workspace) => workspace.id === item.workspace_id) ||
      (item.workspace_name || item.workspace_key
        ? {
            id: item.workspace_id,
            name: item.workspace_name,
            key: item.workspace_key,
          }
        : null)
    );
  }

  function canEditItem() {
    return Boolean(canEdit);
  }

  let displayColumns = $derived.by(() => {
    if (!includeWorkspace) return columns;

    const workspaceColumn = {
      field_identifier: 'workspace',
      field_type: 'workspace',
      display_order: 2,
      width: 3,
    };
    const titleIndex = columns.findIndex(
      (column) => column.field_identifier === 'title',
    );
    const insertionIndex = titleIndex >= 0 ? titleIndex + 1 : 0;
    return [
      ...columns.slice(0, insertionIndex),
      workspaceColumn,
      ...columns.slice(insertionIndex),
    ];
  });

  let tableColumns = $derived([
    ...displayColumns.map((column) => ({
      ...column,
      key: `list-column-${column.field_type}-${column.field_identifier}`,
      label: getListColumnLabel(column, customFieldDefinitions),
      width: getListColumnTableWidth(column),
      slot: 'workItemCell',
    })),
    { key: 'actions', label: '', width: 'w-16' },
  ]);
</script>

<div data-testid={testId}>
  <DataTable
    {data}
    columns={tableColumns}
    keyField="id"
    emptyMessage={emptyMessage}
    emptyDescription={emptyDescription}
    emptyIcon={emptyIcon}
    {actionItems}
    {onRowClick}
    {rowAttrs}
  >
    {#snippet workItemCell(item, column)}
      <ListCellRenderer
        {item}
        column={column}
        workspace={getWorkspace(item)}
        canEdit={canEditItem()}
        {statuses}
        {statusCategories}
        {priorities}
        {milestones}
        {iterations}
        {projects}
        {itemTypes}
        {customFieldDefinitions}
      />
    {/snippet}
  </DataTable>
</div>
