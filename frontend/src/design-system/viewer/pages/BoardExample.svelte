<script>
  import Filter from '@lucide/svelte/icons/filter';
  import Plus from '@lucide/svelte/icons/plus';
  import Button from '../../../lib/components/Button.svelte';
  import SearchInput from '../../../lib/components/SearchInput.svelte';
  import BoardColumn from '../../../lib/features/collections/BoardColumn.svelte';
  import BoardEmptyState from '../../../lib/features/collections/BoardEmptyState.svelte';
  import BoardItemCard from '../../../lib/features/collections/BoardItemCard.svelte';
  import StaticViewBackground from '../../../lib/layout/StaticViewBackground.svelte';
  import ViewHeader from '../../../lib/layout/ViewHeader.svelte';

  const workspace = { id: 1, key: 'WIND', name: 'Windshift' };
  const itemTypes = [{ id: 1, name: 'Task', icon: 'CheckSquare', color: '#2874bb' }];
  const priorities = [
    { id: 1, name: 'High', color: '#dc2626' },
    { id: 2, name: 'Medium', color: '#d97706' },
    { id: 3, name: 'Low', color: '#6b7280' },
  ];
  const labels = [
    { id: 1, name: 'Design', color: '#8b5cf6' },
    { id: 2, name: 'Frontend', color: '#2874bb' },
    { id: 3, name: 'Platform', color: '#0d9488' },
  ];
  const users = [
    { id: 1, first_name: 'Maya', last_name: 'Chen' },
    { id: 2, first_name: 'Noah', last_name: 'Williams' },
    { id: 3, first_name: 'Priya', last_name: 'Shah' },
  ];
  const cardFields = [
    { field_type: 'system', field_identifier: 'priority' },
    { field_type: 'system', field_identifier: 'labels' },
    { field_type: 'system', field_identifier: 'due_date' },
  ];
  const columns = [
    { id: 'todo', name: 'To do', color: '#8993a4', status_ids: [1] },
    { id: 'progress', name: 'In progress', color: '#2874bb', status_ids: [2], wip_limit: 3 },
    { id: 'done', name: 'Done', color: '#16a34a', status_ids: [3] },
  ];
  const items = [
    { id: 248, workspace_key: 'WIND', workspace_item_number: 248, title: 'Polish the new workspace onboarding', status_id: 1, item_type_id: 1, priority_id: 1, label_ids: [1], assignee_id: 1, due_date: '2026-07-24' },
    { id: 251, workspace_key: 'WIND', workspace_item_number: 251, title: 'Define empty states for collection views', status_id: 1, item_type_id: 1, priority_id: 2, label_ids: [1], assignee_id: 2, due_date: '2026-07-26' },
    { id: 255, workspace_key: 'WIND', workspace_item_number: 255, title: 'Add keyboard shortcuts to quick create', status_id: 1, item_type_id: 1, priority_id: 3, label_ids: [2], assignee_id: 3, due_date: '2026-07-30' },
    { id: 242, workspace_key: 'WIND', workspace_item_number: 242, title: 'Unify filters across board and list views', status_id: 2, item_type_id: 1, priority_id: 1, label_ids: [3], assignee_id: 2, due_date: '2026-07-22' },
    { id: 246, workspace_key: 'WIND', workspace_item_number: 246, title: 'Create product launch reporting widget', status_id: 2, item_type_id: 1, priority_id: 2, label_ids: [2], assignee_id: 1, due_date: '2026-07-25' },
    { id: 231, workspace_key: 'WIND', workspace_item_number: 231, title: 'Publish Q3 launch checklist', status_id: 3, item_type_id: 1, priority_id: 2, label_ids: [3], assignee_id: 1, due_date: '2026-07-18' },
    { id: 237, workspace_key: 'WIND', workspace_item_number: 237, title: 'Review accessibility of primary flows', status_id: 3, item_type_id: 1, priority_id: 1, label_ids: [1], assignee_id: 2, due_date: '2026-07-20' },
  ];

  let searchTerm = $state('');
  let showHighPriorityOnly = $state(false);

  const filteredItems = $derived(
    items.filter((item) => {
      const query = searchTerm.trim().toLowerCase();
      const itemLabels = labels.filter((label) => item.label_ids.includes(label.id));
      const matchesQuery =
        !query ||
        item.title.toLowerCase().includes(query) ||
        `${item.workspace_key}-${item.workspace_item_number}`.toLowerCase().includes(query) ||
        itemLabels.some((label) => label.name.toLowerCase().includes(query));
      return matchesQuery && (!showHighPriorityOnly || item.priority_id === 1);
    }),
  );

  function itemsForColumn(column) {
    return filteredItems.filter((item) => column.status_ids.includes(item.status_id));
  }
</script>

<StaticViewBackground contentClass="p-6 min-w-fit" testid="design-system-board-example">
  <div class="mb-8">
    <ViewHeader workspaceName={workspace.name} collection="Product launch" viewName="Board" itemCount={items.length}>
      {#snippet actions()}
        <Button variant="primary" size="small" icon={Plus}>Add item</Button>
      {/snippet}
    </ViewHeader>
  </div>

  <div class="mb-6 flex items-center gap-4">
    <SearchInput
      bind:value={searchTerm}
      placeholder="Search"
      dataTestid="design-system-board-search"
      class="w-72"
    />
    <Button
      variant={showHighPriorityOnly ? 'selected' : 'ghost'}
      icon={Filter}
      dataTestid="design-system-board-priority-filter"
      onclick={() => (showHighPriorityOnly = !showHighPriorityOnly)}
    >
      High priority
    </Button>
  </div>

  <div class="grid min-w-[900px] grid-cols-3 gap-6" data-testid="design-system-board-columns">
    {#each columns as column}
      {@const columnItems = itemsForColumn(column)}
      <BoardColumn
        {column}
        itemCount={columnItems.length}
        wipCount={columnItems.length}
        isOverWip={Boolean(column.wip_limit && columnItems.length > column.wip_limit)}
        statusColumnKey={`example-${column.id}-${column.status_ids[0]}`}
        statusId={column.status_ids[0]}
        onadd={() => {}}
        oncollapse={() => {}}
      >
        {#if columnItems.length === 0}
          <BoardEmptyState />
        {:else}
          <div class="space-y-1">
            {#each columnItems as item (item.id)}
              <BoardItemCard
                {item}
                {workspace}
                {itemTypes}
                {cardFields}
                {priorities}
                {labels}
                {users}
              />
            {/each}
          </div>
        {/if}
      </BoardColumn>
    {/each}
  </div>
</StaticViewBackground>
