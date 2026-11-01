<script>
  import UserPicker from '../../pickers/UserPicker.svelte';
  import AssetPicker from '../../pickers/AssetPicker.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import PortalCustomerPicker from '../../pickers/PortalCustomerPicker.svelte';
  import CustomerOrganisationPicker from '../../pickers/CustomerOrganisationPicker.svelte';
  import LinkingFieldPicker from '../../pickers/LinkingFieldPicker.svelte';
  import Box from '@lucide/svelte/icons/box';
  import Globe from '@lucide/svelte/icons/globe';
  import Building2 from '@lucide/svelte/icons/building-2';
  import Calendar from '@lucide/svelte/icons/calendar';
  import User from '@lucide/svelte/icons/user';
  import Target from '@lucide/svelte/icons/target';
  import Link2 from '@lucide/svelte/icons/link-2';
  import Mail from '@lucide/svelte/icons/mail';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import CheckSquare from '@lucide/svelte/icons/square-check';
  import ColorDot from '../../components/ColorDot.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import TruncatedFieldValue from '../../components/TruncatedFieldValue.svelte';
  import { onDestroy } from 'svelte';
  import { referenceDisplayCache } from '../../stores/referenceDisplayCache.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatCustomFieldDate } from '../../utils/dateFormatter.js';
  import { fieldOptionsObject, parseFieldOptions, resolveOptionLabel, resolveOptionLabels } from '../../utils/optionUtils.js';
  import { safeHref } from '../../utils/sanitize';
  import { customFieldLinkHref } from '../../utils/customFieldLinks.js';
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
    onStartEdit = null, onCancel = null, showSelectedInTrigger = true,
    noPadding = false, itemId = null, users: providedUsers = null, fieldLinks = null,
    onFieldLinksChanged = null,
    optionData = {}, optionLoading = {}, onRequestOptions = null, loadAssetOptions = null,
    displayAlignment = 'start', truncateDisplay = false, displayTestId = undefined,
    // Row mode: the caller passes the field label and the component renders the
    // label/value row used by the item detail sidebar, swapping it for a
    // full-width editor. rowTestIdPrefix builds the row's stable test ids:
    // `<prefix>-<field id>` for the row, `<prefix>-<part>-<field id>` for its
    // label, value, edit and display hooks.
    label = null, rowTestIdPrefix = undefined,
    // Bare display hands layout to the caller: no padding, hover, or rounding.
    bare = false,
    // Self-editing mode: the component owns the display→editor toggle, the way
    // list cells need it. Callers that drive editing externally (item detail
    // sidebar) keep using onStartEdit + readonly and leave this off.
    selfEditing = false
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
      case 'multi_milestone':
        return normalizedMilestoneIDs().map((id) => milestoneName(id) ?? id).join(', ');
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
	  return !((field.field_type === 'multiselect' || field.field_type === 'multi_milestone') && Array.isArray(value) && value.length === 0);
	}

  const rowMode = $derived(label !== null);
  // The row right-aligns and truncates its value; standalone displays keep the
  // caller's layout choices.
  const valueAlignment = $derived(rowMode ? 'end' : displayAlignment);
  const valueTruncate = $derived(rowMode ? true : truncateDisplay);
  // References keep the rich display (avatar, asset tag) inside the row.
  const rowRichDisplay = $derived(['user', 'multi_user', 'asset'].includes(field.field_type));
  const rowValueHref = $derived(rowRichDisplay ? null : customFieldLinkHref(field.field_type, value));

  // Row values are plain text: the per-type icons of the rich display do not
  // fit the sidebar's label/value row.
  function rowDisplayValue() {
    if (!hasDisplayValue()) return t('common.none');
    const v = /** @type {any} */ (value);
    switch (field.field_type) {
      case 'date':
        return formatDateDisplay(v);
      case 'milestone':
      case 'multi_milestone': {
        const entries = field.field_type === 'multi_milestone'
          ? (Array.isArray(v) ? v : [])
          : [v];
        return entries.map((entry) => {
          const id = entry && typeof entry === 'object' ? entry.id : entry;
          return milestoneName(id) ?? id;
        }).join(', ');
      }
      case 'iteration': {
        const iteration = iterations?.find((i) => i.id === parseInt(v, 10));
        return iteration ? iteration.name : v;
      }
      case 'select':
        return field.options ? resolveOptionLabel(field.options, v) : v;
      case 'multiselect': {
        if (!field.options) return v;
        const values = Array.isArray(v)
          ? v
          : typeof v === 'string' && v.includes(',')
            ? v.split(',').map((entry) => entry.trim()).filter(Boolean)
            : [v];
        return resolveOptionLabels(field.options, values).join(', ');
      }
      case 'boolean':
      case 'checkbox':
        return booleanCustomFieldChecked(v) ? t('common.yes') : t('common.no');
      default:
        if (Array.isArray(v)) {
          return v
            .map((entry) => (entry && typeof entry === 'object'
              ? entry.title || entry.name || entry.label || entry.value
              : entry))
            .join(', ');
        }
        if (v && typeof v === 'object') {
          return v.title || v.name || v.label || v.value || JSON.stringify(v);
        }
        return v;
    }
  }

  // Handle keydown for text/number inputs
  function handleKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      commitFromInput(/** @type {HTMLInputElement} */ (event.currentTarget).value);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      handleEditorCancel();
    }
  }

  // Free-form inputs keep focus while typing; commit once focus leaves so a
  // partially typed value is never saved.
  function handleCommitBlur(event) {
    commitFromInput(/** @type {HTMLInputElement | HTMLTextAreaElement} */ (event.currentTarget).value);
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

  function normalizedMilestoneIDs() {
    const raw = /** @type {any} */ (value);
    if (!raw) return [];
    const entries = Array.isArray(raw) ? raw : [raw];
    return entries.map((entry) => {
      if (entry && typeof entry === 'object') return parseInt(entry.id, 10);
      return parseInt(entry, 10);
    }).filter((id) => Number.isFinite(id) && id > 0);
  }

  function milestoneName(id) {
    const milestone = milestones?.find((m) => m.id === parseInt(id, 10));
    return milestone ? milestone.name : null;
  }

  // --- Self-editing plumbing ------------------------------------------------
  // In self-editing mode the component starts in display form and swaps to
  // the editor on click, commit-on-change for single-value types and commit
  // on Enter/blur for free-form inputs, Escape or outside click to cancel.
  let editorOpen = $state(false);
  let lastExitAt = 0;
  // Free-form inputs stage keystrokes in a draft so self-editing surfaces
  // commit once (Enter/blur) instead of on every character.
  let draftValue = $state(null);

  const isSelfEditing = $derived(selfEditing && !disabled && readonly);
  // Booleans edit in place — render the checkbox directly instead of routing
  // through a display→editor swap.
  const renderLiveEditor = $derived(isSelfEditing && isBooleanCustomFieldType(field.field_type));
  // An editor only exists after the user activated the field, either through
  // self-editing or an external onStartEdit/readonly swap, so its picker opens
  // right away. Forms mount every field in editor mode up front and stay closed.
  const pickerAutoOpen = $derived(editorOpen || (onStartEdit !== null && !readonly));
  const editorValue = $derived(draftValue !== null ? draftValue : value);

  function enterEdit() {
    // The blur-commit that exits the editor re-renders the display under the
    // pointer; swallow that stray click so the cell does not re-open.
    if (Date.now() - lastExitAt < 200) return;
    draftValue = null;
    editorOpen = true;
  }

  function exitEditing() {
    if (!editorOpen && draftValue === null) return;
    lastExitAt = Date.now();
    editorOpen = false;
    draftValue = null;
  }

  function handleActivate() {
    if (disabled) return;
    if (onStartEdit) {
      onStartEdit();
      return;
    }
    if (isSelfEditing) enterEdit();
  }

  function handleEditorChange(newValue) {
    onChange(newValue);
    // Multi-value editors stay open so several entries can be picked in a
    // row; everything else returns to display right after the commit.
    const multiValue = ['multi_user', 'multiselect', 'multi_milestone'].includes(field.field_type)
      || (field.field_type === 'asset' && isMultiAssetField);
    if (!multiValue) exitEditing();
  }

  function handleEditorCancel() {
    onCancel?.();
    exitEditing();
  }

  function commitFromInput(rawValue) {
    if (isSelfEditing) {
      // Blur fires even when nothing changed — only commit real edits. Both
      // sides stringify first: inputs hand back strings while stored number
      // or boolean values keep their type, and 5 !== "5" would commit on
      // every untouched blur.
      if (stringifyValue(rawValue) !== stringifyValue(value)) onChange(rawValue);
      exitEditing();
      return;
    }
    onCommit?.(rawValue);
  }

  function stringifyValue(v) {
    if (v === null || v === undefined) return '';
    return String(v);
  }

  // Edit-input keystrokes: free-form fields stage a draft in self-editing
  // mode and keep the live onChange behavior everywhere else.
  function handleEditorInput(newValue) {
    if (!isSelfEditing) {
      onChange(newValue);
      return;
    }
    if (field.field_type === 'date') {
      // A date is committed as a whole — there is no useful partial state.
      handleEditorChange(newValue);
      return;
    }
    draftValue = newValue;
  }

  // Labels a selected value that is missing from a picker's option list.
  // Options load lazily in list cells, so the stored value object or the
  // shared display cache is the only reliable source for the label.
  const storedObjectByID = $derived.by(() => {
    const byID = new Map();
    const raw = /** @type {any} */ (value);
    if (!raw) return byID;
    const entries = Array.isArray(raw) ? raw : [raw];
    entries.forEach((entry) => {
      if (entry && typeof entry === 'object' && entry.id != null) byID.set(String(entry.id), entry);
    });
    return byID;
  });

  function resolveMissingLabel(missingValue) {
    if (missingValue == null || missingValue === '') return '';
    switch (field.field_type) {
      case 'asset': {
        const id = assetID(missingValue);
        const stored = id ? storedObjectByID.get(String(id)) : null;
        if (stored) return assetDisplayName(stored);
        const cached = id ? referenceDisplayCache.getAsset(id) : null;
        if (cached) return assetDisplayName(cached);
        return `Asset #${id ?? missingValue}`;
      }
      case 'portalcustomer':
      case 'customerorganisation': {
        if (typeof missingValue === 'object') return missingValue.name || '';
        const stored = storedObjectByID.get(String(missingValue));
        if (stored?.name) return stored.name;
        const noun = field.field_type === 'portalcustomer' ? 'Customer' : 'Organisation';
        return `${noun} #${missingValue}`;
      }
      case 'user': {
        const user = users.find((u) => u.id === parseInt(missingValue));
        return user ? `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.username : '';
      }
      case 'milestone':
        return milestoneData?.name || '';
      case 'multi_milestone':
        return milestoneName(missingValue) || '';
      case 'iteration': {
        const iteration = iterations?.find((i) => i.id === parseInt(missingValue));
        return iteration ? iteration.name : '';
      }
      case 'select':
        return field.options ? resolveOptionLabel(field.options, missingValue) : '';
      case 'multiselect': {
        if (!field.options) return '';
        const values = Array.isArray(missingValue) ? missingValue : [missingValue];
        return values.map((entry) => resolveOptionLabel(field.options, entry)).filter(Boolean).join(', ');
      }
      default:
        return '';
    }
  }
  // --------------------------------------------------------------------------
</script>

{#snippet readOnlyContent(interactive)}
  {#if hasDisplayValue()}
    {#if field.field_type === 'user'}
      {#if userData}
        <div class="flex min-w-0 items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
          <div class="w-4 h-4 rounded-full bg-ds-accent-blue flex items-center justify-center text-ds-text-inverse text-[9px] font-medium flex-shrink-0">
            {userData.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)}
          </div>
          <span class={valueTruncate ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{userData.name}</span>
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
    {:else if field.field_type === 'multi_milestone'}
      <div class="flex min-w-0 items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <Target class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span class={valueTruncate ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'iteration'}
      <div class="flex items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        {#if iterationData}
          {#if iterationData.is_global}
            <Globe class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          {:else}
            <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          {/if}
        {:else}
          <Calendar class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {/if}
        <span class={valueTruncate ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'asset'}
      <div class="flex min-w-0 items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <Box class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span class={valueTruncate ? 'min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'portalcustomer'}
      <div class="flex items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <User class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'customerorganisation'}
      <div class="flex items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <Building2 class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{renderDisplayValue()}</span>
      </div>
    {:else if field.field_type === 'linking'}
      <div class="flex items-center gap-1 {valueAlignment === 'end' ? 'justify-end' : ''}">
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
    {:else if isBooleanCustomFieldType(field.field_type)}
      <div class="flex items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <CheckSquare class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        <span style="color: var(--ds-text);">{booleanCustomFieldChecked(value) ? t('common.yes') : t('common.no')}</span>
      </div>
    {:else if field.field_type === 'email'}
      <div class="flex items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
        <Mail class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
        {#if interactive}
          <span style="color: var(--ds-text);">{value}</span>
        {:else}
          <a href={`mailto:${value}`} class="hover:underline" style="color: var(--ds-text);">{value}</a>
        {/if}
      </div>
    {:else if field.field_type === 'url'}
      <div class="flex min-w-0 items-center gap-2 {valueAlignment === 'end' ? 'justify-end' : ''}">
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
      <span class={valueTruncate ? 'block min-w-0 truncate' : ''} style="color: var(--ds-text);">{renderDisplayValue()}</span>
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
    <span class={valueTruncate ? 'min-w-0 truncate' : ''} style="color: var(--ds-text-subtle);">{t('items.setField', { field: field.name.toLowerCase() })}</span>
  {:else}
    <span class={valueTruncate ? 'block min-w-0 truncate' : ''} style="color: var(--ds-text-subtle);">{t('items.notSet')}</span>
  {/if}
{/snippet}

{#snippet editorContent()}
  <div class="{disabled ? 'opacity-50 pointer-events-none' : ''}">
    {#if field.field_type === 'milestone'}
      <ItemPicker
        {value}
        items={milestones}
        config={milestoneConfig}
        placeholder={t('pickers.selectMilestone')}
        showUnassigned={true}
        unassignedLabel={t('pickers.noMilestone')}
        autoOpen={pickerAutoOpen}
        class="w-full"
        {disabled}
        {resolveMissingLabel}
        onSelect={(item) => handleEditorChange(item?.id || null)}
        onCancel={handleEditorCancel}
      />
    {:else if field.field_type === 'multi_milestone'}
      <ItemPicker
        value={normalizedMilestoneIDs()}
        values={normalizedMilestoneIDs()}
        items={milestones}
        config={milestoneConfig}
        placeholder={t('pickers.selectMilestones')}
        showUnassigned={false}
        multiSelect={true}
        autoOpen={pickerAutoOpen}
        class="w-full"
        {disabled}
        {resolveMissingLabel}
        onSelect={(selected) => handleEditorChange(
          Array.isArray(selected)
            ? selected.map((entry) => (entry && typeof entry === 'object' ? entry.id : entry)).filter((entry) => entry != null)
            : []
        )}
        onCancel={handleEditorCancel}
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
        users={providedUsers === null ? (users.length ? users : null) : providedUsers}
        loading={optionLoading.users ?? false}
        autoOpen={pickerAutoOpen}
        {resolveMissingLabel}
        onOpen={() => onRequestOptions?.('users')}
        onSelect={(selectedUser) => {
          handleEditorChange(selectedUser ? {
            id: selectedUser.id,
            name: `${selectedUser.first_name} ${selectedUser.last_name}`.trim() || selectedUser.username
          } : null);
        }}
        onCancel={handleEditorCancel}
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
          users={providedUsers === null ? (users.length ? users : null) : providedUsers}
          loading={optionLoading.users ?? false}
          autoOpen={pickerAutoOpen}
          onOpen={() => onRequestOptions?.('users')}
          onSelect={addMultiUser}
          onCancel={handleEditorCancel}
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
        autoOpen={pickerAutoOpen}
        class="w-full"
        {disabled}
        {resolveMissingLabel}
        onSelect={(item) => handleEditorChange(item?.id || null)}
        onCancel={handleEditorCancel}
      />
    {:else if field.field_type === 'asset'}
      {@const assetValue = isMultiAssetField ? normalizedAssetIDs() : (value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null))}
      <AssetPicker
        value={assetValue}
        assetSetId={assetConfig.asset_set_id}
        cqlQuery={assetConfig.cql_query || assetConfig.ql_query}
        placeholder={t('pickers.selectAsset')}
        showUnassigned={!isMultiAssetField}
        autoOpen={pickerAutoOpen}
        multiple={isMultiAssetField}
        class="w-full"
        {disabled}
        {resolveMissingLabel}
        optionLoader={loadAssetOptions
          ? (search) => loadAssetOptions(
              assetConfig.asset_set_id,
              assetConfig.cql_query || assetConfig.ql_query || '',
              search,
            )
          : null}
        onSelect={(asset) => {
          handleEditorChange(asset ? {
            id: asset.id,
            title: asset.title,
            asset_tag: asset.asset_tag || ''
          } : null);
        }}
        onChange={(assets) => handleEditorChange(assets)}
        onCancel={handleEditorCancel}
      />
    {:else if field.field_type === 'portalcustomer'}
      {@const customerValue = value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null)}
      <PortalCustomerPicker
        value={customerValue}
        placeholder="Select portal customer"
        showUnassigned={true}
        class="w-full"
        {disabled}
        autoOpen={pickerAutoOpen}
        {resolveMissingLabel}
        customers={optionData.portalCustomers ?? null}
        loading={optionLoading.portalCustomers ?? false}
        onOpen={() => onRequestOptions?.('portalCustomers')}
        onSelect={(customer) => {
          handleEditorChange(customer ? {
            id: customer.id,
            name: customer.name,
            email: customer.email
          } : null);
        }}
        onCancel={handleEditorCancel}
      />
    {:else if field.field_type === 'customerorganisation'}
      {@const orgValue = value && typeof value === 'object' ? /** @type {any} */ (value).id : (value ?? null)}
      <CustomerOrganisationPicker
        value={orgValue}
        placeholder="Select organisation"
        showUnassigned={true}
        class="w-full"
        {disabled}
        autoOpen={pickerAutoOpen}
        {resolveMissingLabel}
        organisations={optionData.customerOrganisations ?? null}
        loading={optionLoading.customerOrganisations ?? false}
        onOpen={() => onRequestOptions?.('customerOrganisations')}
        onSelect={(org) => {
          handleEditorChange(org ? {
            id: org.id,
            name: org.name
          } : null);
        }}
        onCancel={handleEditorCancel}
      />
    {:else if field.field_type === 'linking'}
      <LinkingFieldPicker
        fieldId={field.id}
        {itemId}
        fieldOptions={field.options}
        readonly={false}
        {disabled}
        links={fieldLinks}
        onChanged={(change) => onFieldLinksChanged?.(change)}
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
        autoOpen={pickerAutoOpen}
        {disabled}
        {resolveMissingLabel}
        onSelect={(item) => handleEditorChange(item ? item.id : null)}
      />
    {:else if field.field_type === 'multiselect'}
      <BasePicker
        value={Array.isArray(value) ? value : []}
        items={parseOptions(field.options)}
        placeholder={t('items.selectField', { field: field.name.toLowerCase() })}
        getValue={(item) => item.id}
        getLabel={(item) => item.label}
        multiple={true}
        autoOpen={pickerAutoOpen}
        {disabled}
        {resolveMissingLabel}
        onChange={(selected) => handleEditorChange(selected)}
      />
    {:else if field.field_type === 'date'}
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="date"
          value={formatDateForInput(editorValue)}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(formatDateFromInput(/** @type {HTMLInputElement} */ (e.target).value))}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          onkeydown={handleKeydown}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if field.field_type === 'textarea'}
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Textarea
          value={editorValue}
          data-testid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(/** @type {HTMLTextAreaElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          rows={3}
          {disabled}
          required={isRequired}
          autofocus
          size="small"
          onblur={handleCommitBlur}
        />
      </div>
    {:else if field.field_type === 'number'}
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="number"
          step="any"
          value={editorValue}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded tabular-nums"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          onblur={handleCommitBlur}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if isBooleanCustomFieldType(field.field_type)}
      <div
        use:clickOutside
        onclickOutside={handleEditorCancel}
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
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="email"
          value={editorValue}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          onblur={handleCommitBlur}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else if field.field_type === 'url'}
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="url"
          value={editorValue}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          onblur={handleCommitBlur}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {:else}
      <!-- Default: text input -->
      <div use:clickOutside onclickOutside={handleEditorCancel}>
        <!-- svelte-ignore a11y_autofocus -->
        <Input
          type="text"
          value={editorValue}
          dataTestid={`custom-field-input-${field.id}`}
          oninput={(e) => handleEditorInput(/** @type {HTMLInputElement} */ (e.target).value)}
          class="w-full px-3 py-2 text-sm hover:bg-ds-background-neutral-hovered focus:outline-none transition-colors bg-transparent border rounded"
          style="background-color: {isDarkMode ? '#1e293b' : 'var(--ds-background-input)'}; border-color: {isDarkMode ? '#475569' : 'var(--ds-border)'}; color: {isDarkMode ? '#e2e8f0' : 'var(--ds-text)'};"
          placeholder={t('items.enterField', { field: field.name.toLowerCase() })}
          onkeydown={handleKeydown}
          onblur={handleCommitBlur}
          {disabled}
          required={isRequired}
          autofocus
        />
      </div>
    {/if}
  </div>
{/snippet}

{#snippet rowValue()}
  {#if rowRichDisplay}
    <button
      type="button"
      class="flex w-full min-w-0 items-center justify-end gap-2 text-right whitespace-nowrap overflow-hidden"
      onclick={handleActivate}
      data-testid={`${rowTestIdPrefix}-display-${field.id}`}
      {disabled}
    >
      {@render readOnlyContent(true)}
    </button>
  {:else}
    <TruncatedFieldValue
      value={rowDisplayValue()}
      href={rowValueHref}
      onactivate={rowValueHref ? null : handleActivate}
      {disabled}
      subtle={!hasDisplayValue()}
      testId={rowValueHref
        ? `${rowTestIdPrefix}-value-${field.id}`
        : `${rowTestIdPrefix}-edit-${field.id}`}
    />
  {/if}
{/snippet}

{#if readonly && !editorOpen && !renderLiveEditor}
  {#if rowMode}
    <!-- Row mode: label on the left, value on the right; the row owns its
         padding and hover chrome. -->
    <div
      class="hover-bg group flex w-full min-w-0 items-center justify-between gap-2 rounded px-2 py-1.5 text-sm transition-colors"
      data-testid={`${rowTestIdPrefix}-${field.id}`}
    >
      <button
        type="button"
        onclick={handleActivate}
        data-testid={rowValueHref
          ? `${rowTestIdPrefix}-edit-${field.id}`
          : `${rowTestIdPrefix}-label-${field.id}`}
        class="max-w-[45%] shrink-0 truncate rounded-sm text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-default"
        style="outline-color: var(--ds-border-focused);"
        {disabled}
      >
        {@render label(field.name)}
      </button>
      <div class="min-w-0 flex-1 text-right">
        {@render rowValue()}
      </div>
    </div>
  {:else}
    <div>
      {#if (onStartEdit || isSelfEditing) && !disabled}
        <button
          type="button"
          class="flex w-full min-w-0 items-center gap-2 {displayAlignment === 'end' ? 'justify-end text-right' : 'justify-start text-left'} {truncateDisplay ? 'whitespace-nowrap overflow-hidden' : ''} {bare || noPadding ? '' : 'px-3'} {bare ? '' : 'py-2'} text-sm {bare ? '' : 'hover:bg-ds-background-neutral-hovered transition-colors rounded'}"
          onclick={handleActivate}
          data-testid={displayTestId}
        >
          {@render readOnlyContent(true)}
        </button>
      {:else}
        <div
          class="min-w-0 {displayAlignment === 'end' ? 'text-right' : ''} {truncateDisplay ? 'whitespace-nowrap overflow-hidden' : ''} {bare || noPadding ? '' : 'px-3'} {bare ? '' : 'py-2'} text-sm {disabled ? 'opacity-50' : ''}"
          data-testid={displayTestId}
        >
          {@render readOnlyContent(false)}
        </div>
      {/if}
    </div>
  {/if}
{:else if rowMode}
  <div class="w-full" data-testid={`${rowTestIdPrefix}-${field.id}`}>
    {@render editorContent()}
  </div>
{:else}
  <!-- Edit mode -->
  {@render editorContent()}
{/if}
