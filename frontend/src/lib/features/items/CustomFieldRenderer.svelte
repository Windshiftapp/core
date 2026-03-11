<script>
  import UserPicker from '../../pickers/UserPicker.svelte';
  import AssetPicker from '../../pickers/AssetPicker.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import PersonalLabelCombobox from '../../pickers/PersonalLabelCombobox.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import PortalCustomerPicker from '../../pickers/PortalCustomerPicker.svelte';
  import CustomerOrganisationPicker from '../../pickers/CustomerOrganisationPicker.svelte';
  import { Box, Globe, Building2, Calendar, User, Target } from 'lucide-svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';

  // Helper to parse field options
  function parseOptions(optionsStr) {
    if (!optionsStr) return [];
    return optionsStr.startsWith('[')
      ? JSON.parse(optionsStr)
      : optionsStr.split(',').map(opt => opt.trim());
  }

  let users = $state([]);
  let usersLoading = $state(false);
  let usersLoaded = $state(false);

  async function loadUsers() {
    if (usersLoading || usersLoaded) return;
    usersLoading = true;
    try {
      users = await api.getUsers() || [];
      usersLoaded = true;
    } catch (e) {
      console.error('Failed to load users:', e);
      users = [];
    } finally {
      usersLoading = false;
    }
  }

  // Click outside action
  function clickOutside(node) {
    const handleClick = (event) => {
      if (!node.contains(event.target)) {
        node.dispatchEvent(new CustomEvent('clickOutside'));
      }
    };

    document.addEventListener('click', handleClick, true);

    return {
      destroy() {
        document.removeEventListener('click', handleClick, true);
      }
    };
  }

  let {
    field, value = '', onChange = () => {}, milestones = [], iterations = [],
    isDarkMode = false, required = false, readonly = true, disabled = false,
    onStartEdit = null, onCancel = null, showSelectedInTrigger = true, autoOpenPickers = true,
    noPadding = false
  } = $props();

  const isRequired = $derived(required || field.required || field.is_required);

  // Milestone config for ItemPicker
  const milestoneConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: null
  };

  // Iteration config for ItemPicker
  const iterationConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: (item) => item.is_global ? 'Global' : 'Team'
  };

  // Helper to format date for display
  function formatDateDisplay(dateValue) {
    if (!dateValue) return '';
    return formatDateShort(dateValue) || dateValue;
  }

  // Helper to format date for input[type="date"] (YYYY-MM-DD)
  function formatDateForInput(dateValue) {
    if (!dateValue) return '';
    try {
      const date = new Date(dateValue);
      return date.toISOString().split('T')[0];
    } catch (e) {
      return '';
    }
  }

  // Helper to format date from input back to ISO string
  function formatDateFromInput(inputValue) {
    if (!inputValue) return '';
    try {
      return new Date(inputValue).toISOString();
    } catch (e) {
      return inputValue;
    }
  }

  // Helper to render value text for display
  function renderDisplayValue() {
    if (value === null || value === undefined || value === '') {
      return null;
    }

    switch (field.field_type) {
      case 'user':
        if (typeof value === 'object' && value.name) {
          return value.name;
        }
        return value;
      case 'iteration':
        if (value && iterations) {
          const iteration = iterations.find(i => i.id === parseInt(value));
          return iteration ? iteration.name : value;
        }
        return value;
      case 'milestone':
        if (value && milestones) {
          const milestone = milestones.find(m => m.id === parseInt(value));
          return milestone ? milestone.name : value;
        }
        return value;
      case 'asset':
        if (typeof value === 'object' && value.title) {
          return value.asset_tag ? `${value.asset_tag} - ${value.title}` : value.title;
        }
        return `Asset #${value}`;
      case 'portalcustomer':
        if (typeof value === 'object' && value.name) {
          return value.name;
        }
        return `Customer #${value}`;
      case 'customerorganisation':
        if (typeof value === 'object' && value.name) {
          return value.name;
        }
        return `Organisation #${value}`;
      case 'select':
      case 'multiselect':
        if (field.options) {
          try {
            let options;
            if (field.options.startsWith('[')) {
              options = JSON.parse(field.options);
            } else {
              options = field.options.split(',').map(opt => opt.trim());
            }
            if (field.field_type === 'multiselect') {
              const selectedValues = Array.isArray(value) ? value : value.split(',').map(v => v.trim());
              return selectedValues.filter(v => options.includes(v)).join(', ');
            }
            return options.includes(value) ? value : value;
          } catch (e) {
            return value;
          }
        }
        return value;
      case 'number':
        const num = parseFloat(value);
        return isNaN(num) ? value : num.toString();
      case 'date':
        return formatDateDisplay(value);
      default:
        return value;
    }
  }

  // Handle keydown for text/number inputs
  function handleKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      // Trigger save by calling onChange with current value
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

  // Load users when we need to look up a user by ID
  $effect(() => {
    if (readonly && field.field_type === 'user' && value && typeof value !== 'object') {
      loadUsers();
    }
  });

  // Reactive user data computation
  const userData = $derived((() => {
    if (field.field_type !== 'user' || !value) return null;
    // If it's already an object with name, use it
    if (typeof value === 'object' && value.name) return value;
    // If it's an ID, look up the user
    const userId = typeof value === 'object' ? value.id : value;
    const user = users.find(u => u.id === parseInt(userId));
    if (user) {
      return {
        id: user.id,
        name: `${user.first_name} ${user.last_name}`.trim() || user.username
      };
    }
    return null;
  })());

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

{#if readonly}
  <!-- Read-only display mode -->
  <div>
    {#if onStartEdit && !disabled}
      <button
        type="button"
        class="w-full flex items-center gap-2 justify-start {noPadding ? '' : 'px-3'} py-2 text-sm hover:bg-gray-50 transition-colors text-left rounded"
        onclick={handleClick}
      >
        {#if value !== null && value !== undefined && value !== ''}
          {#if field.field_type === 'user'}
            <!-- Display user with avatar -->
            {#if userData}
              <div class="w-4 h-4 rounded-full bg-blue-500 flex items-center justify-center text-white text-[9px] font-medium flex-shrink-0">
                {userData.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
              </div>
              <span style="color: var(--ds-text);">{userData.name}</span>
            {:else if usersLoading}
              <span style="color: var(--ds-text-subtle);">{t('common.loading')}</span>
            {:else}
              <span style="color: var(--ds-text-subtle);">{t('common.unknownUser')}</span>
            {/if}
          {:else if field.field_type === 'milestone'}
            <!-- Display milestone with color dot -->
            {#if milestoneData}
              <ColorDot color={milestoneData.category_color || '#9CA3AF'} />
              <span style="color: var(--ds-text);">{milestoneData.name}</span>
            {:else}
              <Target class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
              <span style="color: var(--ds-text-subtle);">{t('items.setField', { field: field.name.toLowerCase() })}</span>
            {/if}
          {:else if field.field_type === 'iteration'}
            <!-- Display iteration with icon -->
            {#if iterationData}
              {#if iterationData.is_global}
                <Globe class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
              {:else}
                <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
              {/if}
            {:else}
              <Calendar class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            {/if}
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else if field.field_type === 'asset'}
            <!-- Display asset with icon -->
            <Box class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else if field.field_type === 'portalcustomer'}
            <!-- Display portal customer with icon -->
            <User class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else if field.field_type === 'customerorganisation'}
            <!-- Display customer organisation with icon -->
            <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else if field.field_type === 'combobox'}
            <!-- Display labels as chips/tags -->
            <div class="flex items-center gap-1 flex-wrap">
              {#each getComboboxLabels(value) as labelName}
                <span class="inline-flex items-center px-2 py-0.5 bg-blue-100 text-blue-800 text-xs rounded-full">
                  {labelName}
                </span>
              {/each}
            </div>
          {:else if field.field_type === 'number'}
            <span class="tabular-nums" style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else}
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {/if}
        {:else}
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
          {/if}
          <span style="color: var(--ds-text-subtle);">{t('items.setField', { field: field.name.toLowerCase() })}</span>
        {/if}
      </button>
    {:else}
      <!-- Static display (no click handler or disabled) -->
      <div class="{noPadding ? '' : 'px-3'} py-2 text-sm {disabled ? 'opacity-50' : ''}">
        {#if value !== null && value !== undefined && value !== ''}
          {#if field.field_type === 'user'}
            {#if userData}
              <div class="flex items-center gap-2">
                <div class="w-4 h-4 rounded-full bg-blue-500 flex items-center justify-center text-white text-[9px] font-medium flex-shrink-0">
                  {userData.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
                </div>
                <span style="color: var(--ds-text);">{userData.name}</span>
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
              {:else}
                <span style="color: var(--ds-text-subtle);">{t('items.notSet')}</span>
              {/if}
            </div>
          {:else if field.field_type === 'iteration'}
            <div class="flex items-center gap-2">
              {#if iterationData}
                {#if iterationData.is_global}
                  <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {:else}
                  <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                {/if}
              {:else}
                <Calendar class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              {/if}
              <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
            </div>
          {:else if field.field_type === 'asset'}
            <div class="flex items-center gap-2">
              <Box class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
            </div>
          {:else if field.field_type === 'portalcustomer'}
            <div class="flex items-center gap-2">
              <User class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
            </div>
          {:else if field.field_type === 'customerorganisation'}
            <div class="flex items-center gap-2">
              <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
            </div>
          {:else if field.field_type === 'combobox'}
            <div class="flex items-center gap-1 flex-wrap">
              {#each getComboboxLabels(value) as labelName}
                <span class="inline-flex items-center px-2 py-0.5 bg-blue-100 text-blue-800 text-xs rounded-full">
                  {labelName}
                </span>
              {/each}
            </div>
          {:else if field.field_type === 'number'}
            <span class="tabular-nums" style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {:else}
            <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
          {/if}
        {:else}
          <span style="color: var(--ds-text-subtle);">{t('items.notSet')}</span>
        {/if}
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
        on:select={(e) => onChange(e.detail?.id || null)}
        on:cancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'user'}
      {@const userValue = value && typeof value === 'object' ? value.id : value}
      <UserPicker
        value={userValue}
        placeholder={t('pickers.selectUser')}
        showUnassigned={true}
        {showSelectedInTrigger}
        autoOpen={autoOpenPickers}
        class="w-full"
        {disabled}
        onSelect={(selectedUser) => {
          onChange(selectedUser ? {
            id: selectedUser.id,
            name: `${selectedUser.first_name} ${selectedUser.last_name}`.trim() || selectedUser.username
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
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
        on:select={(e) => onChange(e.detail?.id || null)}
        on:cancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'asset'}
      {@const assetConfig = field.options ? JSON.parse(field.options) : {}}
      {@const assetValue = value && typeof value === 'object' ? value.id : value}
      <AssetPicker
        value={assetValue}
        assetSetId={assetConfig.asset_set_id}
        cqlQuery={assetConfig.cql_query}
        placeholder={t('pickers.selectAsset')}
        showUnassigned={true}
        autoOpen={autoOpenPickers}
        class="w-full"
        {disabled}
        onSelect={(asset) => {
          onChange(asset ? {
            id: asset.id,
            title: asset.title,
            asset_tag: asset.asset_tag || ''
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'portalcustomer'}
      {@const customerValue = value && typeof value === 'object' ? value.id : value}
      <PortalCustomerPicker
        value={customerValue}
        placeholder="Select portal customer"
        showUnassigned={true}
        class="w-full"
        {disabled}
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
      {@const orgValue = value && typeof value === 'object' ? value.id : value}
      <CustomerOrganisationPicker
        value={orgValue}
        placeholder="Select organisation"
        showUnassigned={true}
        class="w-full"
        {disabled}
        onSelect={(org) => {
          onChange(org ? {
            id: org.id,
            name: org.name
          } : null);
        }}
        onCancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'combobox'}
      <PersonalLabelCombobox
        {value}
        placeholder={t('items.selectOrCreateLabels')}
        class="w-full"
        userId={null}
        {disabled}
        on:select={(e) => {
          const labelArray = e.detail.value || [];
          onChange(labelArray.join(','));
        }}
        on:cancel={() => onCancel?.()}
      />
    {:else if field.field_type === 'select'}
      <BasePicker
        {value}
        items={parseOptions(field.options)}
        placeholder={t('items.selectField', { field: field.name.toLowerCase() })}
        showUnassigned={true}
        unassignedLabel={t('items.selectField', { field: field.name.toLowerCase() })}
        getValue={(item) => item}
        getLabel={(item) => item}
        {disabled}
        onSelect={(item) => onChange(item || '')}
      />
    {:else if field.field_type === 'multiselect'}
      <BasePicker
        value={value ? value.split(',').filter(v => v) : []}
        items={parseOptions(field.options)}
        placeholder={t('items.selectField', { field: field.name.toLowerCase() })}
        getValue={(item) => item}
        getLabel={(item) => item}
        multiple={true}
        {disabled}
        onChange={(selected) => onChange(selected.join(','))}
      />
    {:else if field.field_type === 'date'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          type="date"
          value={formatDateForInput(value)}
          oninput={(e) => onChange(formatDateFromInput(e.target.value))}
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
        <textarea
          {value}
          oninput={(e) => onChange(e.target.value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          rows="3"
          {disabled}
          required={isRequired}
          autofocus
        ></textarea>
      </div>
    {:else if field.field_type === 'number'}
      <div use:clickOutside onclickOutside={() => onCancel?.()}>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          type="number"
          step="any"
          {value}
          oninput={(e) => onChange(e.target.value)}
          class="w-full px-3 py-2 text-sm hover:bg-gray-50 focus:outline-none transition-colors bg-transparent border rounded tabular-nums"
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
        <input
          type="text"
          {value}
          oninput={(e) => onChange(e.target.value)}
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
