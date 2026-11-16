<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { get } from '../api/core.js';
  import { onDestroy, onMount } from 'svelte';
  import Shield from '@lucide/svelte/icons/shield';
  import { t } from '../stores/i18n.svelte.js';
  import { objectDisplayDescription, objectDisplayName } from '../utils/systemLabels.js';

  let {
    value = $bindable(null),
    id = undefined,
    placeholder = '',
    label = '',
    disabled = false,
    multiple = false,
    roles: suppliedRoles = undefined,
    rolesState = 'ready',
    class: className = '',
    onSelect = () => {},
    onChange = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectRole'));

  const roles = createAsyncLoader(() => get('/workspace-roles'));

  onMount(() => { if (suppliedRoles === undefined) roles.load(); });
  onDestroy(() => roles.dispose());
</script>

<BasePicker
  bind:value
  {id}
  items={suppliedRoles ?? roles.data}
  loading={suppliedRoles === undefined ? roles.loading : rolesState === 'loading'}
  error={suppliedRoles === undefined ? roles.error : rolesState === 'error' ? t('errors.failedToLoad') : null}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  {multiple}
  class={className}
   searchFields={['display_name', 'name', 'display_description', 'description']}
  getValue={(role) => role?.id}
   getLabel={(role) => objectDisplayName(role, 'workspace_role')}
  onSelect={(item) => onSelect(item)}
  onChange={(roleIDs) => onChange(roleIDs)}
  onCancel={() => onCancel()}
  optionTestid={(option) => `role-picker-option-${option.value}`}
>
  {#snippet itemSnippet({ item: role, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Role Icon -->
      <div class="flex-shrink-0">
        <div class="w-8 h-8 rounded-full flex items-center justify-center" style="background: var(--ds-interactive); color: var(--ds-text-inverse);">
          <Shield class="w-4 h-4" />
        </div>
      </div>

      <!-- Role Info -->
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{objectDisplayName(role, 'workspace_role')}</span>
        {#if role.description}
          <span class="text-sm truncate" style="color: var(--ds-text-subtle);">{objectDisplayDescription(role, 'workspace_role')}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
