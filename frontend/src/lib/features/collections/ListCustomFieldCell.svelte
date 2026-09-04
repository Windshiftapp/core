<script>
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import { collectionEditorOptions } from '../../stores/collectionEditorOptions.svelte.js';
  import { User, Target } from '@lucide/svelte';
  import { milestonePickerConfig as milestoneConfig } from '../../pickers/pickerConfigs.js';
  import UserCellValue from './UserCellValue.svelte';
  import IterationCellEditor from './IterationCellEditor.svelte';
  import MilestoneCellValue from './MilestoneCellValue.svelte';

  let {
    field,
    value = null,
    canEdit = false,
    milestones = [],
    iterations = [],
    users = [],
    editorOptions = null,
    workspaceId = null,
    itemId = null,
    fieldLinks = [],
    onFieldLinksChanged = null,
    onChange = (_value) => {}
  } = $props();

</script>

{#if field.field_type === 'milestone'}
  {@const milestone = value ? [...(editorOptions?.milestones ?? []), ...milestones].find(m => m.id === parseInt(value)) : null}
  {#if canEdit}
    <ItemPicker
      {value}
      items={editorOptions?.milestones ?? milestones}
      config={milestoneConfig}
      placeholder={field.name}
      showUnassigned={true}
      unassignedLabel="No {field.name.toLowerCase()}"
      allowClear={true}
      loading={editorOptions?.loading?.milestones ?? false}
      onOpen={() => editorOptions && collectionEditorOptions.load(workspaceId, 'milestones')}
      onSelect={(item) => onChange(item?.id || null)}
    >
      {#snippet children()}
        {#if milestone}
          <MilestoneCellValue {milestone} interactive />
        {:else}
          <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
            <Target class="w-4 h-4" />
            {field.name}
          </span>
        {/if}
      {/snippet}
    </ItemPicker>
  {:else}
    {#if milestone}
      <MilestoneCellValue {milestone} />
    {:else}
      <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
    {/if}
  {/if}

{:else if field.field_type === 'iteration'}
  {@const iteration = value ? [...(editorOptions?.iterations ?? []), ...iterations].find(i => i.id === parseInt(value)) : null}
  <IterationCellEditor
    {canEdit}
    {value}
    {iteration}
    items={editorOptions?.iterations ?? iterations}
    loading={editorOptions?.loading?.iterations ?? false}
    placeholder={field.name}
    unassignedLabel="No {field.name.toLowerCase()}"
    selectPrompt={field.name}
    onOpen={() => editorOptions && collectionEditorOptions.load(workspaceId, 'iterations')}
    onSelect={(item) => onChange(item?.id || null)}
  />

{:else if field.field_type === 'user'}
  {@const userValue = value && typeof value === 'object' ? value.id : value}
  {@const assignee = userValue ? [...(editorOptions?.users ?? []), ...users].find(u => u.id === parseInt(userValue)) : null}
  {#if canEdit}
    <UserPicker
      value={userValue}
      placeholder={field.name}
      showUnassigned={true}
      users={editorOptions?.users ?? users}
      loading={editorOptions?.loading?.users ?? false}
      onOpen={() => editorOptions && collectionEditorOptions.load(workspaceId, 'users')}
      onSelect={(selectedUser) => {
        onChange(selectedUser ? {
          id: selectedUser.id,
          name: `${selectedUser.first_name} ${selectedUser.last_name}`.trim() || selectedUser.username
        } : null);
      }}
    >
      {#snippet children()}
        {#if assignee}
          <UserCellValue user={assignee} interactive />
        {:else if value && typeof value === 'object' && value.name}
          <UserCellValue fallbackName={value.name} fallbackAvatar interactive />
        {:else}
          <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
            <User class="w-4 h-4" />
            {field.name}
          </span>
        {/if}
      {/snippet}
    </UserPicker>
  {:else}
    {#if assignee}
      <UserCellValue user={assignee} />
    {:else if value && typeof value === 'object' && value.name}
      <UserCellValue fallbackName={value.name} fallbackAvatar />
    {:else}
      <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
    {/if}
  {/if}

{:else}
  <!-- Non-picker types: delegate to CustomFieldRenderer -->
  <CustomFieldRenderer
    {field}
    {value}
    readonly={!canEdit}
    disabled={!canEdit}
    autoOpenPickers={false}
    {milestones}
    {iterations}
    users={editorOptions?.loaded?.users ? editorOptions.users : users}
    optionData={editorOptions ?? {}}
    optionLoading={editorOptions?.loading ?? {}}
    onRequestOptions={(field) => editorOptions && collectionEditorOptions.load(workspaceId, field)}
    loadAssetOptions={(assetSetId, cqlQuery, search) => collectionEditorOptions.loadAssets(workspaceId, assetSetId, cqlQuery, search)}
    {itemId}
    {fieldLinks}
    {onFieldLinksChanged}
    onChange={(val) => onChange(val)}
  />
{/if}
