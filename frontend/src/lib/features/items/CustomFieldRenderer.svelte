<script>
  import UserPicker from '../../pickers/UserPicker.svelte';
  import AssetPicker from '../../pickers/AssetPicker.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import PersonalLabelCombobox from '../../pickers/PersonalLabelCombobox.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import PortalCustomerPicker from '../../pickers/PortalCustomerPicker.svelte';
  import CustomerOrganisationPicker from '../../pickers/CustomerOrganisationPicker.svelte';
  import LinkingFieldPicker from '../../pickers/LinkingFieldPicker.svelte';
  import { Box, Globe, Building2, Calendar, User, Target, Link2, Mail, ExternalLink, CheckSquare } from '@lucide/svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { onDestroy } from 'svelte';
  import { referenceDisplayCache } from '../../stores/referenceDisplayCache.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatCustomFieldDate } from '../../utils/dateFormatter.js';
  import { fieldOptionsObject, parseFieldOptions, resolveOptionLabel, resolveOptionLabels } from '../../utils/optionUtils.js';
  import { safeHref } from '../../utils/sanitize';
  import { booleanCustomFieldChecked, isBooleanCustomFieldType } from '../../utils/customFieldTypes.js';
  import {
    milestonePickerConfig as milestoneConfig,
    iterationPickerConfig as iterationConfig,
  } from '../../pickers/pickerConfigs.js';
  import { clickOutside } from '../../actions/clickOutside.js';

  // Helper to parse field options into [{id, label}] items
  function parseOptions(optionsStr) {
    const { items } = parseFieldOptions(optionsStr);
    return items;
  }

  async function loadUsers() {
    if (providedUsers !== null) return;
    await referenceDisplayCache.loadUsers();
  }

  let {
    field, value = $bindable(''), onChange = () => {}, onCommit = null, milestones = [], iterations = [],
    isDarkMode = false, required = false, readonly = true, disabled = false,
    onStartEdit = null, onCancel = null, showSelectedInTrigger = true, autoOpenPickers = true,
    noPadding = false, itemId = null, users: providedUsers = null, fieldLinks = null,
    onFieldLinksChanged = null,
    optionData = {}, optionLoading = {}, onRequestOptions = null, loadAssetOptions = null,
    displayAlignment = 'start', truncateDisplay = false, displayTestId = undefined
  } = $props();

  const users = $derived(providedUsers ?? referenceDisplayCache.users);
  const usersLoading = $derived(providedUsers === null && referenceDisplayCache.usersLoading);
  const displayHydration = new AbortController();
  onDestroy(() => displayHydration.abort());

  const isRequired = $derived(required || field.required || field.is_required);

  // Custom-field dates are calendar days, not moments in time — keep them as
  // YYYY-MM-DD strings end-to-end to avoid timezone drift.
  function formatDateDisplay(dateValue) {
    if (!dateValue) return '';
    return formatCustomFieldDate(dateValue) || dateValue;
  }

  function formatDateForInput(dateValue) {
    if (!dateValue) return '';
    return typeof dateValue === 'string' ? dateValue.slice(0, 10) : '';
  }

  function formatDateFromInput(inputValue) {
    return inputValue || '';
  }

  function parseAssetConfig() {
    try { return fieldOptionsObject(field.options); }
    catch { return {}; }
  }

  const assetConfig = $derived(parseAssetConfig());
  const isMultiAssetField = $derived(field.field_type === 'asset' && assetConfig.multi === true);
  function assetID(asset) {
    if (!asset) return null;
    const raw = asset && typeof asset === 'object' ? asset.id : asset;
    const id = parseInt(raw, 10);
    return Number.isFinite(id) && id > 0 ? id : null;
  }

  function assetIDsForLookup() {
    if (field.field_type !== 'asset') return [];
    const raw = /** @type {any} */ (value);
    if (!raw) return [];
    const entries = Array.isArray(raw) ? raw : [raw];
    return entries
      .filter((entry) => !(entry && typeof entry === 'object' && entry.title))
      .map(assetID)
      .filter((id) => id !== null);
  }

  async function loadAssetDisplayValues() {
    await referenceDisplayCache.loadAssets(assetIDsForLookup(), {
      signal: displayHydration.signal,
    });
  }

  function assetDisplayName(asset) {
    const id = assetID(asset);
    const resolved = id ? referenceDisplayCache.getAsset(id) : null;
    const displayAsset = resolved || asset;
    if (displayAsset && typeof displayAsset === 'object') {
      if (displayAsset.title) return displayAsset.asset_tag ? `${displayAsset.asset_tag} - ${displayAsset.title}` : displayAsset.title;
      if (displayAsset.id) return `Asset #${displayAsset.id}`;
    }
    return `Asset #${asset}`;
  }

  function normalizedAssetIDs() {
    const raw = /** @type {any} */ (value);
    if (!raw) return [];
    const entries = Array.isArray(raw) ? raw : [raw];
    return entries.map((entry) => {
      if (entry && typeof entry === 'object') return parseInt(entry.id, 10);
      return parseInt(entry, 10);
    }).filter((id) => Number.isFinite(id) && id > 0);
  }

  // Helper to render value text for display
  function renderDisplayValue() {
    if (value === null || value === undefined || value === '') {
      return null;
    }
    // After null guard above, value is non-null
    const v = /** @type {any} */ (value);

    switch (field.field_type) {
      case 'user':
        if (typeof v === 'object' && v.name) {
          return v.name;
        }
        return v;
      case 'multi_user':
        return multiUserNames().join(', ');
      case 'iteration':
        if (v && iterations) {
          const iteration = iterations.find(i => i.id === parseInt(v));
          return iteration ? iteration.name : v;
        }
        return v;
      case 'milestone':
        if (v && milestones) {
          const milestone = milestones.find(m => m.id === parseInt(v));
          return milestone ? milestone.name : v;
        }
        return v;
      case 'asset':
        if (Array.isArray(v)) {
          return v.map(assetDisplayName).join(', ');
        }
        return assetDisplayName(v);
      case 'portalcustomer':
        if (typeof v === 'object' && v.name) {
          return v.name;
        }
		return `Customer #${typeof v === 'object' ? v.id : v}`;
      case 'customerorganisation':
        if (typeof v === 'object' && v.name) {
          return v.name;
        }
		return `Organisation #${typeof v === 'object' ? v.id : v}`;
      case 'select':
      case 'multiselect':
        if (field.options) {
          if (field.field_type === 'multiselect') {
			const values = Array.isArray(v)
			  ? v
			  : typeof v === 'string' && v.includes(',')
				? v.split(',').map(item => item.trim()).filter(Boolean)
				: [v];
			return resolveOptionLabels(field.options, values).join(', ');
          }
          return resolveOptionLabel(field.options, v);
        }
        return v;
      case 'boolean':
      case 'checkbox':
        return booleanCustomFieldChecked(v) ? t('common.yes') : t('common.no');
      case 'number':
        const num = parseFloat(v);
        return isNaN(num) ? v : num.toString();
      case 'date':
        return formatDateDisplay(v);
      default:
        return v;
    }
  }

	function hasDisplayValue() {
	  if (value === null || value === undefined || value === '') return false;
	  return !(field.field_type === 'multiselect' && Array.isArray(value) && value.length === 0);
	}

  // Handle keydown for text/number inputs
  function handleKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      onCommit?.(/** @type {HTMLInputElement} */ (event.currentTarget).value);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      onCancel?.();
    }
  }

  // Handle click on read mode to start editing
  function handleClick() {
    if (!disabled && onStartEdit) {
      onStartEdit();
    }
  }

  // Get iteration data for icon rendering
  const iterationData = $derived(
    field.field_type === 'iteration' && value && iterations
      ? iterations.find(i => i.id === parseInt(value))
      : null
  );

  // Load users when we need to look up user IDs
  $effect(() => {
    if (readonly && field.field_type === 'user' && value && typeof value !== 'object') {
      loadUsers();
    }
    if (field.field_type === 'multi_user' && normalizedMultiUserIDs().length > 0) {
      loadUsers();
    }
    if (readonly && field.field_type === 'asset' && assetIDsForLookup().length > 0) {
      loadAssetDisplayValues();
    }
  });

  // Reactive user data computation
  const userData = $derived((() => {
    if (field.field_type !== 'user' || !value) return null;
    const v = /** @type {any} */ (value);
    // If it's already an object with name, use it
    if (typeof v === 'object' && v.name) return v;
    // If it's an ID, look up the user
    const userId = typeof v === 'object' ? v.id : v;
    const user = users.find(u => u.id === parseInt(userId));
    if (user) {
      return {
        id: user.id,
        name: `${user.first_name} ${user.last_name}`.trim() || user.username
      };
    }
    return null;
  })());

  function normalizedMultiUserIDs() {
    const raw = /** @type {any} */ (value);
    if (!raw) return [];
    const entries = Array.isArray(raw) ? raw : [raw];
    return entries.map((entry) => {
      if (typeof entry === 'object') return parseInt(entry.id ?? entry.user_id, 10);
      return parseInt(entry, 10);
    }).filter((id) => Number.isFinite(id) && id > 0);
  }

  function multiUserNames() {
    const raw = /** @type {any} */ (value);
    if (!raw) return [];
    const entries = Array.isArray(raw) ? raw : [raw];
    return entries.map((entry) => {
      if (typeof entry === 'object' && entry.name) return entry.name;
      const id = typeof entry === 'object' ? parseInt(entry.id ?? entry.user_id, 10) : parseInt(entry, 10);
      const user = users.find((u) => u.id === id);
      return user ? `${user.first_name} ${user.last_name}`.trim() || user.username : `#${id}`;
    }).filter(Boolean);
  }

  function multiUserObjects() {
    return normalizedMultiUserIDs().map((id) => {
      const user = users.find((u) => u.id === id);
      return {
        id,
        name: user ? `${user.first_name} ${user.last_name}`.trim() || user.username : `#${id}`
      };
    });
  }

  function addMultiUser(selectedUser) {
    if (!selectedUser) return;
    const ids = normalizedMultiUserIDs();
    if (!ids.includes(selectedUser.id)) ids.push(selectedUser.id);
    onChange(ids);
  }

  function removeMultiUser(id) {
    onChange(normalizedMultiUserIDs().filter((existing) => existing !== id));
  }

  // Reactive milestone data computation
  const milestoneData = $derived((() => {
    if (field.field_type !== 'milestone' || !value) return null;
    const milestone = milestones.find(m => m.id === parseInt(value));
    return milestone || null;
  })());

  // Get combobox labels array
  function getComboboxLabels(val) {
    if (!val) return [];
    return val.split(',').map(v => v.trim()).filter(v => v);
  }
</script>

{#snippet readOnlyContent(interactive)}
  {#if hasDisplayValue()}
    {#if field.field_type === 'user'}
      {#if userData}
        <div class="flex min-w-0 items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
          <div class="w-4 h-4 rounded-full bg-blue-500 flex items-center justify-center text-white text-[9px] font-medium flex-shrink-0">
            {userData.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
          </div>
          <span class={truncateDisplay ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{userData.name}</span>
        </div>
      {:else if usersLoading}
        <span style="color: var(--ds-text-subtle);">{t('common.loading')}</span>
      {:else}
        <span style="color: var(--ds-text-subtle);">{t('common.unknownUser')}</span>
      {/if}
    {:else if field.field_type === 'milestone'}
      <div class="flex items-center gap-2">
        {#if milestoneData}
          <ColorDot color={milestoneData.category_color || '#9CA3AF'} />
          <span style="color: var(--ds-text);">{milestoneData.name}</span>
        {:else if interactive}
          <Target class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <span style="color: var(--ds-text-subtle);">{t('items.setField', { field: field.name.toLowerCase() })}</span>
        {:else}
          <span style="color: var(--ds-text-subtle);">{t('items.notSet')}</span>
        {/if}
      </div>
    {:else if field.field_type === 'iteration'}
      <div class="flex items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        {#if iterationData}
          {#if iterationData.is_global}
            <Globe class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          {:else}
            <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          {/if}
        {:else}
          <Calendar class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {/if}
        <span class={truncateDisplay ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'asset'}
      <div class="flex min-w-0 items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <Box class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span class={truncateDisplay ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'portalcustomer'}
      <div class="flex items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <User class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'customerorganisation'}
      <div class="flex items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'linking'}
      <div class="flex items-center gap-1 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <Link2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">
          {#if Array.isArray(value) && value.length > 0}
            {value.length} linked
          {:else if !Array.isArray(value) && value && typeof value === 'object'}
            1 linked
          {:else}
            —
          {/if}
        </span>
      </div>
    {:else if field.field_type === 'combobox'}
      <div class="flex items-center gap-1 flex-wrap {displayAlignment === 'end' ? 'justify-end' : ''}">
        {#each getComboboxLabels(value) as labelName}
          <span class="inline-flex items-center px-2 py-0.5 bg-blue-100 text-blue-800 text-xs rounded-full">
            {labelName}
          </span>
        {/each}
      </div>
    {:else if isBooleanCustomFieldType(field.field_type)}
      <div class="flex items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <CheckSquare class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{booleanCustomFieldChecked(value) ? t('common.yes') : t('common.no')}</span>
      </div>
    {:else if field.field_type === 'email'}
      <div class="flex items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <Mail class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {#if interactive}
          <span style="color: var(--ds-text);">{value}</span>
        {:else}
          <a href={`mailto:${value}`} class="hover:underline" style="color: var(--ds-text);">{value}</a>
        {/if}
      </div>
    {:else if field.field_type === 'url'}
      <div class="flex min-w-0 items-center gap-2 {displayAlignment === 'end' ? 'justify-end' : ''}">
        <ExternalLink class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {#if interactive}
          <span style="color: var(--ds-text);" class="truncate">{value}</span>
        {:else}
          <a href={safeHref(value)} target="_blank" rel="noopener noreferrer" class="hover:underline truncate" style="color: var(--ds-text);">{value}</a>
        {/if}
      </div>
    {:else if field.field_type === 'number'}
      <span class="tabular-nums" style="color: var(--ds-text);">{renderDisplayValue()}</span>
    {:else}
      <span class={truncateDisplay ? 'block min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
    {/if}
  {:else if interactive}
    {#if field.field_type === 'user'}
      <User class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'milestone'}
      <Target class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'asset'}
      <Box class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'portalcustomer'}
      <User class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'customerorganisation'}
      <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if isBooleanCustomFieldType(field.field_type)}
      <CheckSquare class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'email'}
      <Mail class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {:else if field.field_type === 'url'}
      <ExternalLink class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
    {/if}
    <span class={truncateDisplay ? 'min-w-0 truncate' : ''} style="color: var(--ds-text-subtle);">{t('items.setField', { field: field.name.toLowerCase() })}</span>
  {:else}
    <span class={truncateDisplay ? 'block min-w-0 truncate' : ''} style="color: var(--ds-text-subtle);">{t('items.notSet')}</span>
  {/if}
{/snippet}

{#if readonly}
  <div>
    {#if onStartEdit && !disabled}
      <button
        type="button"
        class="flex w-full min-w-0 items-center gap-2 {displayAlignment === 'end' ? 'justify-end text-right' : 'justify-start text-left'} {truncateDisplay ? 'whitespace-nowrap overflow-hidden' : ''} {noPadding ? '' : 'px-3'} py-2 text-sm hover:bg-gray-50 transition-colors rounded"
        onclick={handleClick}
        data-testid={displayTestId}
      >
        {@render readOnlyContent(true)}
      </button>
    {:else}
      <div
        class="min-w-0 {displayAlignment === 'end' ? 'text-right' : ''} {truncateDisplay ? 'whitespace-nowrap overflow-hidden' : ''} {noPadding ? '' : 'px-3'} py-2 text-sm {disabled ? 'opacity-50' : ''}"
        data-testid={displayTestId}
      >
        {@render readOnlyContent(false)}
      </div>
    {/if}
  </div>
{:else}
  <!-- Edit mode -->
  <div class="{disabled ? 'opacity-50 pointer-events-none' : ''}">
    {#if field.field_type === 'milestone'}
      <ItemPicker
        {value}
        items={milestones}
        config={milestoneConfig}
        placeholder={t('pickers.selectMilestone')}
        showUnassigned={true}
        unassignedLabel={t('pickers.noMilestone')}
        autoOpen={autoOpenPickers}
        class="w-full"
        {disabled}
        onSelect={(item) => onChange(item?.id || null)}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'user'}
      {@const userValue = value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null)}
      <UserPicker
        value={userValue}
        placeholder={t('pickers.selectUser')}
        showUnassigned={true}
        {showSelectedInTrigger}
        class="w-full"
        {disabled}
        users={providedUsers}
        loading={optionLoading.users ?? false}
        onOpen={() => onRequestOptions?.('users')}
        onSelect={(selectedUser) => {
          onChange(selectedUser ? {
            id: selectedUser.id,
            name: `${selectedUser.first_name} ${selectedUser.last_name}`.trim() || selectedUser.username
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'multi_user'}
      <div class="space-y-2">
        {#if multiUserObjects().length > 0}
          <div class="flex flex-wrap gap-1.5">
            {#each multiUserObjects() as selectedUser (selectedUser.id)}
              <span class="inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs" style="background: var(--ds-background-neutral); color: var(--ds-text);">
                {selectedUser.name}
                <button type="button" class="hover:opacity-70" onclick={() => removeMultiUser(selectedUser.id)} aria-label={`Remove ${selectedUser.name}`}>×</button>
              </span>
            {/each}
          </div>
        {/if}
        <UserPicker
          value={null}
          placeholder={t('pickers.selectUser')}
          showUnassigned={false}
          showSelectedInTrigger={false}
          class="w-full"
          {disabled}
          users={providedUsers}
          loading={optionLoading.users ?? false}
          onOpen={() => onRequestOptions?.('users')}
          onSelect={addMultiUser}
          onCancel={() => onCancel?.()}
        />
      </div>
    {:else if field.field_type === 'iteration'}
      <ItemPicker
        {value}
        items={iterations}
        config={iterationConfig}
        placeholder={t('items.selectIteration')}
        showUnassigned={true}
        unassignedLabel={t('items.noIteration')}
        autoOpen={autoOpenPickers}
        class="w-full"
        {disabled}
        onSelect={(item) => onChange(item?.id || null)}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'asset'}
      {@const assetValue = isMultiAssetField ? normalizedAssetIDs() : (value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null))}
      <AssetPicker
        value={assetValue}
        assetSetId={assetConfig.asset_set_id}
        cqlQuery={assetConfig.cql_query || assetConfig.ql_query}
        placeholder={t('pickers.selectAsset')}
        showUnassigned={!isMultiAssetField}
        autoOpen={autoOpenPickers}
        multiple={isMultiAssetField}
        class="w-full"
        {disabled}
        optionLoader={loadAssetOptions
          ? (search) => loadAssetOptions(
              assetConfig.asset_set_id,
              assetConfig.cql_query || assetConfig.ql_query || '',
              search,
            )
          : null}
        onSelect={(asset) => {
          onChange(asset ? {
            id: asset.id,
            title: asset.title,
            asset_tag: asset.asset_tag || ''
          } : null);
        }}
        onChange={(assets) => onChange(assets)}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'portalcustomer'}
      {@const customerValue = value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null)}
      <PortalCustomerPicker
        value={customerValue}
        placeholder="Select portal customer"
        showUnassigned={true}
        class="w-full"
        {disabled}
        customers={optionData.portalCustomers ?? null}
        loading={optionLoading.portalCustomers ?? false}
        onOpen={() => onRequestOptions?.('portalCustomers')}
        onSelect={(customer) => {
          onChange(customer ? {
            id: customer.id,
            name: customer.name,
            email: customer.email
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'customerorganisation'}
      {@const orgValue = value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null)}
      <CustomerOrganisationPicker
        value={orgValue}
        placeholder="Select organisation"
        showUnassigned={true}
        class="w-full"
        {disabled}
        organisations={optionData.customerOrganisations ?? null}
        loading={optionLoading.customerOrganisations ?? false}
        onOpen={() => onRequestOptions?.('customerOrganisations')}
        onSelect={(org) => {
          onChange(org ? {
            id: org.id,
            name: org.name
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'linking'}
      <LinkingFieldPicker
        fieldId={field.id}
        {itemId}
        fieldOptions={field.options}
        {readonly}
        {disabled}
        links={fieldLinks}
        onChanged={(change) => onFieldLinksChanged?.(change)}
      />
    {:else if field.field_type === 'combobox'}
      <PersonalLabelCombobox
        {value}
        placeholder={t('items.selectOrCreateLabels')}
        class="w-full"
        userId={null}
        {disabled}
        labels={optionData.personalLabels ?? null}
        loading={optionLoading.personalLabels ?? false}
        onOpen={() => onRequestOptions?.('personalLabels')}
        onSelect={(result) => {
          const labelArray = result.value || [];
          onChange(labelArray.join(','));
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'select'}
      <BasePicker
        {value}
        items={parseOptions(field.options)}
        placeholder={t('items.selectField', { field: field.name.toLowerCase() })}
        showUnassigned={true}
        unassignedLabel={t('items.selectField', { field: field.name.toLowerCase() })}
        getValue={(item) => item.id}
        getLabel={(item) => item.label}
        {disabled}
        onSelect={(item) => onChange(item ? item.id : null)}
      />
    {:else if field.field_type === 'multiselect'}
      <BasePicker
        value={Array.isArray(value) ? value : []}
        items={parseOptions(field.options)}
        placeholder={t('items.selectField', { field: field.name.toLowerCase() })}
        getValue={(item) => item.id}
        getLabel={(item) => item.label}
        multiple={true}
        {disabled}
        onChange={(selected) => onChange(selected)}
      />
    {:else if field.field_type === 'date'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="date"
          value={formatDateForInput(value)}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(formatDateFromInput(/** @type {HTMLInputElement} */ (e.target).value))}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if field.field_type === 'textarea'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Textarea
          {value}
          data-testid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(/** @type {HTMLTextAreaElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          rows={3}
          {disabled}
          required={isRequired}
          autofocus
          size="small"
        />
      </div>
    {:else if field.field_type === 'number'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="number"
          step="any"
          {value}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded tabular-nums"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if isBooleanCustomFieldType(field.field_type)}
      <div
        use:clickOutside
        onclickOutside={() => onCancel?.()}
        class="px-3 py-2"
        data-testid={`custom-field-input-${field.id}`}
      >
        <Checkbox
          checked={booleanCustomFieldChecked(value)}
          {disabled}
          onchange={(checked) => onChange(checked)}
        />
      </div>
    {:else if field.field_type === 'email'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="email"
          {value}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if field.field_type === 'url'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="url"
          {value}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else}
      <!-- Default: text input -->
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="text"
          {value}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => onChange(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {/if}
  </div>
{/if}
