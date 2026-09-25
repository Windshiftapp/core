<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onDestroy, onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    placeholder = '',
    disabled = false,
    class: className = '',
    showSelectedInTrigger = true,
    children: customTrigger = null,
    teams = null,
    searchTestid = 'team-picker-search',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectTeam'));

  const loader = createAsyncLoader(() => api.teams.getAll());
  onMount(() => {
    if (!teams) loader.load();
  });
  onDestroy(() => loader.dispose());

  let teamList = $derived(teams ?? loader.data ?? []);
  let selectedTeam = $derived(teamList.find((team) => team.id === value) || null);
</script>

<BasePicker
  bind:value
  items={teamList}
  loading={teams === null ? loader.loading : false}
  placeholder={resolvedPlaceholder}
  {disabled}
  allowClear={true}
  {showSelectedInTrigger}
  class={className}
  searchFields={['name', 'description']}
  getValue={(team) => team?.id}
  getLabel={(team) => team?.name ?? ''}
  {searchTestid}
  optionTestid={(opt) => `team-picker-option-${opt.value}`}
  onSelect={(team) => onSelect(team)}
  onCancel={() => onCancel()}
>
  {#snippet children()}
    {#if customTrigger}
      {@render customTrigger()}
    {:else}
      <div
        aria-disabled={disabled}
        class="relative w-full flex items-center justify-between gap-2 px-3 py-2 rounded text-sm transition-colors"
        style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        style:opacity={disabled ? 0.5 : 1}
        style:cursor={disabled ? 'not-allowed' : 'pointer'}
        data-testid="team-picker-trigger"
      >
        <div class="flex items-center gap-2 flex-1 min-w-0">
          {#if selectedTeam && showSelectedInTrigger}
            <span
              class="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0"
              style="background-color: {selectedTeam.color || '#9ca3af'};"
            ></span>
            <span class="truncate">{selectedTeam.name}</span>
          {:else}
            <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
          {/if}
        </div>
      </div>
    {/if}
  {/snippet}
</BasePicker>
