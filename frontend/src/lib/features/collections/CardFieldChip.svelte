<script>
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import Clock from '@lucide/svelte/icons/clock';
  import CornerLeftUp from '@lucide/svelte/icons/corner-left-up';
  import Chip from '../../components/Chip.svelte';
  import { formatDate, formatDateOnly, formatDateShort, formatStatusAge } from '../../utils/dateFormatter.js';
  import { resolveOptionLabel, resolveOptionLabels } from '../../utils/optionUtils.js';
  import { durationToString } from '../../utils/timeUtils.js';
  import { booleanCustomFieldChecked, isBooleanCustomFieldType } from '../../utils/customFieldTypes.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Renders the chip(s) for a single configured board card field. Centralising
  // this here keeps the board configuration surface (CARD_SELECTABLE_FIELDS) and
  // the on-card rendering from drifting apart — every selectable field rendered
  // in the chip row has a branch below. Footer fields (due_date, sla) are
  // filtered out by BoardItemCard and rendered there instead.
  let {
    cardField,
    item,
    priorities = [],
    statuses = [],
    iterations = [],
    projects = [],
    labels = [],
    customFieldDefinitions = [],
    users = [],
  } = $props();

  let customFieldId = $derived(
    cardField.field_type === 'custom'
      ? parseInt(cardField.field_identifier.replace('custom_field_', ''))
      : null
  );
  let customFieldDef = $derived(
    customFieldId != null ? customFieldDefinitions.find(d => d.id === customFieldId) : null
  );
  let customFieldValue = $derived(
    customFieldId != null
      ? (item.custom_field_values?.[customFieldId] ?? item.custom_field_values?.[String(customFieldId)])
      : null
  );
  let itemLabels = $derived.by(() => {
    if (Array.isArray(item.labels) && item.labels.length > 0) {
      return item.labels;
    }
    return (item.label_ids || [])
      .map((labelId) => labels.find((label) => label.id === labelId))
      .filter(Boolean);
  });
  // Resolve user-type custom field values to display names.
  let customFieldUserNames = $derived.by(() => {
    if (!customFieldDef || !customFieldValue) return [];
    if (customFieldDef.field_type === 'user') {
      const v = /** @type {any} */ (customFieldValue);
      if (typeof v === 'object' && v.name) return [v.name];
      const uid = typeof v === 'object' ? parseInt(v.id ?? v.user_id, 10) : parseInt(v, 10);
      const u = users.find((u) => u.id === uid);
      return u ? [`${u.first_name} ${u.last_name}`.trim() || u.username] : [];
    }
    if (customFieldDef.field_type === 'multi_user') {
      const raw = /** @type {any} */ (customFieldValue);
      if (!raw) return [];
      const entries = Array.isArray(raw) ? raw : [raw];
      return entries.map((entry) => {
        if (typeof entry === 'object' && entry.name) return entry.name;
        const id = typeof entry === 'object' ? parseInt(entry.id ?? entry.user_id, 10) : parseInt(entry, 10);
        const u = users.find((u) => u.id === id);
        return u ? `${u.first_name} ${u.last_name}`.trim() || u.username : null;
      }).filter(Boolean);
    }
    return [];
  });
</script>

{#if cardField.field_type === 'system'}
  {#if cardField.field_identifier === 'priority' && item.priority_id}
    {@const prio = priorities.find(p => p.id === item.priority_id)}
    {#if prio}
      <Chip appearance="metadata" dotColor={prio.color} title="Priority">
        {prio.name}
      </Chip>
    {/if}
  {:else if cardField.field_identifier === 'due_date' && item.due_date}
    <Chip appearance="metadata" icon={CalendarDays} title="Due date">
      {formatDateOnly(item.due_date)}
    </Chip>
  {:else if cardField.field_identifier === 'start_date' && item.start_date}
    <Chip appearance="metadata" icon={CalendarDays} title="Start date">
      Start: {formatDateOnly(item.start_date)}
    </Chip>
  {:else if cardField.field_identifier === 'end_date' && item.end_date}
    <Chip appearance="metadata" icon={CalendarDays} title="End date">
      End: {formatDateOnly(item.end_date)}
    </Chip>
  {:else if cardField.field_identifier === 'story_points' && item.story_points != null}
    <Chip appearance="metadata" title="Story points">
      {item.story_points} pts
    </Chip>
  {:else if cardField.field_identifier === 'estimate' && item.estimate_minutes != null}
    <Chip appearance="metadata" title="Estimate">
      {durationToString(item.estimate_minutes, { withDays: true })}
    </Chip>
  {:else if cardField.field_identifier === 'milestone' && (item.milestones?.length ?? 0) > 0}
    {#each item.milestones as ms (ms.id)}
      <Chip appearance="metadata" dotColor={ms.category_color || '#6b7280'} title="Milestone">
        {ms.name}
      </Chip>
    {/each}
  {:else if cardField.field_identifier === 'iteration' && item.iteration_id}
    {@const iter = iterations.find(i => i.id === item.iteration_id)}
    {#if iter}
      <Chip appearance="metadata" title="Iteration">
        {iter.name}
      </Chip>
    {/if}
  {:else if cardField.field_identifier === 'labels' && itemLabels.length > 0}
    {#each itemLabels.slice(0, 3) as label (label.id)}
      <Chip appearance="metadata" dotColor={label.color || '#6b7280'} title="Label">
        {label.name}
      </Chip>
    {/each}
    {#if itemLabels.length > 3}
      <Chip appearance="metadata" title="Additional labels">
        +{itemLabels.length - 3}
      </Chip>
    {/if}
  {:else if cardField.field_identifier === 'status' && item.status_id}
    {@const st = statuses.find(s => s.id === item.status_id)}
    {#if st}
      <Chip appearance="metadata" dotColor={st.color || st.category_color || '#6b7280'} title="Status">
        {st.name}
      </Chip>
    {/if}
  {:else if cardField.field_identifier === 'created_at' && item.created_at}
    <Chip appearance="metadata" icon={CalendarDays} title="Created">
      {formatDateShort(item.created_at)}
    </Chip>
  {:else if cardField.field_identifier === 'project' && item.project_id}
    {@const proj = projects.find(p => p.id === item.project_id)}
    {#if proj}
      <Chip appearance="metadata" title="Project">
        {proj.name}
      </Chip>
    {/if}
  {:else if cardField.field_identifier === 'parent' && item.parent_id}
    {@const parentKey = item.parent_workspace_item_number != null && item.workspace_key
      ? `${item.workspace_key}-${item.parent_workspace_item_number}`
      : null}
    <Chip
      appearance="metadata"
      icon={CornerLeftUp}
      class="max-w-[12rem]"
      title={item.parent_title || parentKey || 'Parent'}
    >
      {#if parentKey}
        <span class="font-mono flex-shrink-0">{parentKey}</span>
      {/if}
      {#if item.parent_title}
        <span class="truncate">{item.parent_title}</span>
      {/if}
    </Chip>
  {:else if cardField.field_identifier === 'time_in_status' && item.status_since}
    {@const age = formatStatusAge(item.status_since)}
    {#if age}
      <Chip
        appearance="metadata"
        icon={Clock}
        title="In current status since {formatDate(item.status_since)}"
      >
        {age}
      </Chip>
    {/if}
  {/if}
{:else if cardField.field_type === 'custom'}
  {#if customFieldDef && customFieldValue != null}
    {#if customFieldDef.field_type === 'date'}
      <Chip appearance="metadata" icon={CalendarDays} title={customFieldDef.name}>
        {formatDateOnly(customFieldValue)}
      </Chip>
    {:else if customFieldDef.field_type === 'select' && customFieldDef.options}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {resolveOptionLabel(customFieldDef.options, customFieldValue) || customFieldValue}
      </Chip>
    {:else if customFieldDef.field_type === 'multiselect' && customFieldDef.options}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {resolveOptionLabels(customFieldDef.options, customFieldValue).join(', ') || customFieldValue}
      </Chip>
    {:else if isBooleanCustomFieldType(customFieldDef.field_type)}
      {@const checked = booleanCustomFieldChecked(customFieldValue)}
      <span data-testid={`board-card-custom-field-${customFieldId}`}>
        <Chip appearance="metadata" title={`${customFieldDef.name}: ${checked ? t('common.yes') : t('common.no')}`}>
          {customFieldDef.name}: {checked ? t('common.yes') : t('common.no')}
        </Chip>
      </span>
    {:else if customFieldDef.field_type === 'number'}
	  {@const numericValue = parseFloat(String(customFieldValue))}
      <Chip appearance="metadata" title={customFieldDef.name}>
		{Number.isFinite(numericValue) ? numericValue : String(customFieldValue)}
      </Chip>
    {:else if customFieldDef.field_type === 'user' && customFieldUserNames.length > 0}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {customFieldUserNames[0]}
      </Chip>
    {:else if customFieldDef.field_type === 'multi_user' && customFieldUserNames.length > 0}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {customFieldUserNames[0]}{#if customFieldUserNames.length > 1} +{customFieldUserNames.length - 1}{/if}
      </Chip>
    {:else if customFieldDef.field_type === 'multi_milestone'}
      {@const milestoneCount = Array.isArray(customFieldValue) ? customFieldValue.length : 0}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {milestoneCount} {t('pickers.milestones')}
      </Chip>
    {:else if customFieldDef.field_type === 'url' && String(customFieldValue).trim()}
      <Chip appearance="metadata" title={customFieldValue}>
        {String(customFieldValue).length > 30 ? String(customFieldValue).slice(0, 30) + '…' : customFieldValue}
      </Chip>
    {:else if customFieldDef.field_type === 'asset'}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {#if typeof customFieldValue === 'object' && customFieldValue}
          {@const a = /** @type {any} */ (customFieldValue)}
          {#if Array.isArray(a)}
            {a.length} asset{#if a.length !== 1}s{/if}
          {:else}
            {a.asset_tag || a.title || `#${a.id}`}
          {/if}
        {:else}
          #{customFieldValue}
        {/if}
      </Chip>
    {:else if typeof customFieldValue === 'string' && customFieldValue.length > 40}
      <Chip appearance="metadata" title={customFieldValue}>
        {customFieldValue.slice(0, 40)}…
      </Chip>
    {:else}
      <Chip appearance="metadata" title={customFieldDef.name}>
        {customFieldValue}
      </Chip>
    {/if}
  {/if}
{/if}
