<script>
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import { Calendar, User, Target, Globe, Building2 } from 'lucide-svelte';

  let {
    field,
    value = null,
    canEdit = false,
    milestones = [],
    iterations = [],
    users = [],
    onChange = (_value) => {}
  } = $props();

  const milestoneConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: null
  };

  const iterationConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: (item) => item.is_global ? 'Global' : 'Team'
  };
</script>

{#if field.field_type === 'milestone'}
  {@const milestone = value ? milestones.find(m => m.id === parseInt(value)) : null}
  {#if canEdit}
    <ItemPicker
      {value}
      items={milestones}
      config={milestoneConfig}
      placeholder={field.name}
      showUnassigned={true}
      unassignedLabel="No {field.name.toLowerCase()}"
      allowClear={true}
      onSelect={(item) => onChange(item?.id || null)}
    >
      {#snippet children()}
        {#if milestone}
          <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
            <ColorDot color={milestone.category_color || '#9CA3AF'} />
            {milestone.name}
          </span>
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
      <span class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
        <ColorDot color={milestone.category_color || '#9CA3AF'} />
        {milestone.name}
      </span>
    {:else}
      <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
    {/if}
  {/if}

{:else if field.field_type === 'iteration'}
  {@const iteration = value ? iterations.find(i => i.id === parseInt(value)) : null}
  {#if canEdit}
    <ItemPicker
      {value}
      items={iterations}
      config={iterationConfig}
      placeholder={field.name}
      showUnassigned={true}
      unassignedLabel="No {field.name.toLowerCase()}"
      allowClear={true}
      onSelect={(item) => onChange(item?.id || null)}
    >
      {#snippet children()}
        {#if iteration}
          <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
            {#if iteration.is_global}
              <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {:else}
              <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {/if}
            {iteration.name}
          </span>
        {:else}
          <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
            <Calendar class="w-4 h-4" />
            {field.name}
          </span>
        {/if}
      {/snippet}
    </ItemPicker>
  {:else}
    {#if iteration}
      <span class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
        {#if iteration.is_global}
          <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        {:else}
          <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        {/if}
        {iteration.name}
      </span>
    {:else}
      <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
    {/if}
  {/if}

{:else if field.field_type === 'user'}
  {@const userValue = value && typeof value === 'object' ? value.id : value}
  {@const assignee = userValue ? users.find(u => u.id === parseInt(userValue)) : null}
  {#if canEdit}
    <UserPicker
      value={userValue}
      placeholder={field.name}
      showUnassigned={true}
      onSelect={(selectedUser) => {
        onChange(selectedUser ? {
          id: selectedUser.id,
          name: `${selectedUser.first_name} ${selectedUser.last_name}`.trim() || selectedUser.username
        } : null);
      }}
    >
      {#snippet children()}
        {#if assignee}
          <div class="flex items-center gap-2 cursor-pointer">
            <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
              {(assignee.first_name?.[0] || '') + (assignee.last_name?.[0] || '') || assignee.username?.[0]?.toUpperCase() || '?'}
            </div>
            <span class="text-sm truncate" style="color: var(--ds-text);">
              {assignee.first_name} {assignee.last_name}
            </span>
          </div>
        {:else if value && typeof value === 'object' && value.name}
          <div class="flex items-center gap-2 cursor-pointer">
            <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
              {value.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
            </div>
            <span class="text-sm truncate" style="color: var(--ds-text);">
              {value.name}
            </span>
          </div>
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
      <div class="flex items-center gap-2">
        <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
          {(assignee.first_name?.[0] || '') + (assignee.last_name?.[0] || '') || assignee.username?.[0]?.toUpperCase() || '?'}
        </div>
        <span class="text-sm truncate" style="color: var(--ds-text);">
          {assignee.first_name} {assignee.last_name}
        </span>
      </div>
    {:else if value && typeof value === 'object' && value.name}
      <div class="flex items-center gap-2">
        <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
          {value.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
        </div>
        <span class="text-sm truncate" style="color: var(--ds-text);">
          {value.name}
        </span>
      </div>
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
    onChange={(val) => onChange(val)}
  />
{/if}
