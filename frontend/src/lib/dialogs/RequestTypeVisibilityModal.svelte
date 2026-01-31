<script>
  import { Shield, X, Users, Building2, Check, Info } from 'lucide-svelte';
  import Modal from './Modal.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = false,
    requestType = null,
    isDarkMode = false,
    onSaved = () => {},
    onClose = () => {}
  } = $props();

  let visibilityMode = $state('everyone'); // 'everyone' | 'restricted'
  let selectedGroupIds = $state([]);
  let selectedOrgIds = $state([]);
  let groups = $state([]);
  let organisations = $state([]);
  let loading = $state(false);
  let saving = $state(false);
  let error = $state(null);

  // Load groups and organisations when modal opens
  $effect(() => {
    if (isOpen && requestType) {
      loadData();
    }
  });

  async function loadData() {
    loading = true;
    error = null;
    try {
      // Load groups and organisations in parallel
      const [groupsData, orgsData] = await Promise.all([
        api.groups.getAll(),
        api.time.customers.getAll()
      ]);
      groups = groupsData || [];
      organisations = orgsData || [];

      // Initialize from request type
      if (requestType) {
        const hasGroupRestrictions = requestType.visibility_group_ids?.length > 0;
        const hasOrgRestrictions = requestType.visibility_org_ids?.length > 0;

        if (hasGroupRestrictions || hasOrgRestrictions) {
          visibilityMode = 'restricted';
          selectedGroupIds = requestType.visibility_group_ids || [];
          selectedOrgIds = requestType.visibility_org_ids || [];
        } else {
          visibilityMode = 'everyone';
          selectedGroupIds = [];
          selectedOrgIds = [];
        }
      }
    } catch (err) {
      console.error('Failed to load data:', err);
      error = err.message || 'Failed to load groups and organisations';
    } finally {
      loading = false;
    }
  }

  function handleModeChange(mode) {
    visibilityMode = mode;
    if (mode === 'everyone') {
      selectedGroupIds = [];
      selectedOrgIds = [];
    }
  }

  function toggleGroup(groupId) {
    if (selectedGroupIds.includes(groupId)) {
      selectedGroupIds = selectedGroupIds.filter(id => id !== groupId);
    } else {
      selectedGroupIds = [...selectedGroupIds, groupId];
    }
  }

  function toggleOrg(orgId) {
    if (selectedOrgIds.includes(orgId)) {
      selectedOrgIds = selectedOrgIds.filter(id => id !== orgId);
    } else {
      selectedOrgIds = [...selectedOrgIds, orgId];
    }
  }

  async function handleSave() {
    if (!requestType) return;

    saving = true;
    error = null;
    try {
      const groupIds = visibilityMode === 'everyone' ? [] : selectedGroupIds;
      const orgIds = visibilityMode === 'everyone' ? [] : selectedOrgIds;

      await api.requestTypes.updateVisibility(requestType.id, { groupIds, orgIds });
      onSaved();
      onClose();
    } catch (err) {
      console.error('Failed to save visibility:', err);
      error = err.message || 'Failed to save visibility settings';
    } finally {
      saving = false;
    }
  }
</script>

<Modal
  {isOpen}
  onClose={onClose}
  title={t('portal.visibility.title')}
  size="md"
>
  <div class="p-6">
    {#if loading}
      <div class="flex items-center justify-center py-8">
        <Spinner />
      </div>
    {:else}
      {#if error}
        <div class="mb-4 p-3 rounded bg-red-50 text-red-600 text-sm" style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'};">
          {error}
        </div>
      {/if}

      <!-- Request Type Info -->
      <div class="mb-6 flex items-center gap-3">
        <div
          class="w-10 h-10 rounded flex items-center justify-center"
          style="background-color: {requestType?.color || '#6b7280'};"
        >
          <Shield class="w-5 h-5 text-white" />
        </div>
        <div>
          <div class="font-medium" style="color: var(--ds-text);">{requestType?.name || 'Request Type'}</div>
          <div class="text-sm" style="color: var(--ds-text-subtle);">{t('portal.visibility.configureAccess')}</div>
        </div>
      </div>

      <!-- Visibility Mode Selection -->
      <div class="space-y-3 mb-6">
        <button
          type="button"
          onclick={() => handleModeChange('everyone')}
          class="w-full flex items-start gap-3 p-3 rounded border-2 transition-all text-left"
          style="border-color: {visibilityMode === 'everyone' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'}; background-color: {visibilityMode === 'everyone' ? (isDarkMode ? 'rgba(37, 99, 235, 0.1)' : '#eff6ff') : 'transparent'};"
        >
          <div class="w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5"
            style="border-color: {visibilityMode === 'everyone' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'};">
            {#if visibilityMode === 'everyone'}
              <div class="w-2.5 h-2.5 rounded-full" style="background-color: var(--ds-interactive, #2563eb);"></div>
            {/if}
          </div>
          <div class="flex-1">
            <div class="font-medium" style="color: var(--ds-text);">{t('portal.visibility.everyone')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">{t('portal.visibility.everyoneDesc')}</div>
          </div>
        </button>

        <button
          type="button"
          onclick={() => handleModeChange('restricted')}
          class="w-full flex items-start gap-3 p-3 rounded border-2 transition-all text-left"
          style="border-color: {visibilityMode === 'restricted' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'}; background-color: {visibilityMode === 'restricted' ? (isDarkMode ? 'rgba(37, 99, 235, 0.1)' : '#eff6ff') : 'transparent'};"
        >
          <div class="w-5 h-5 rounded-full border-2 flex items-center justify-center mt-0.5"
            style="border-color: {visibilityMode === 'restricted' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'};">
            {#if visibilityMode === 'restricted'}
              <div class="w-2.5 h-2.5 rounded-full" style="background-color: var(--ds-interactive, #2563eb);"></div>
            {/if}
          </div>
          <div class="flex-1">
            <div class="font-medium" style="color: var(--ds-text);">{t('portal.visibility.restricted')}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">{t('portal.visibility.restrictedDesc')}</div>
          </div>
        </button>
      </div>

      <!-- Groups and Organisations Selection (only when restricted) -->
      {#if visibilityMode === 'restricted'}
        <div class="space-y-4">
          <!-- Info hint about OR logic -->
          <div class="flex items-start gap-2 p-3 rounded" style="background-color: {isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#eff6ff'};">
            <Info class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-interactive, #2563eb);" />
            <span class="text-sm" style="color: var(--ds-text-subtle);">{t('portal.visibility.orLogicHint')}</span>
          </div>

          <!-- Internal Groups -->
          <div>
            <div class="flex items-center gap-2 mb-2">
              <Users class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              <span class="text-sm font-medium" style="color: var(--ds-text);">{t('portal.visibility.internalGroups')}</span>
            </div>
            {#if groups.length === 0}
              <div class="text-sm p-3 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-surface-raised);">
                {t('portal.visibility.noGroupsAvailable')}
              </div>
            {:else}
              <div class="border rounded divide-y" style="border-color: var(--ds-border);">
                {#each groups as group}
                  <button
                    type="button"
                    onclick={() => toggleGroup(group.id)}
                    class="w-full flex items-center gap-3 px-3 py-2 text-left transition-all hover:bg-black/5"
                  >
                    <div
                      class="w-5 h-5 rounded border-2 flex items-center justify-center"
                      style="border-color: {selectedGroupIds.includes(group.id) ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'}; background-color: {selectedGroupIds.includes(group.id) ? 'var(--ds-interactive, #2563eb)' : 'transparent'};"
                    >
                      {#if selectedGroupIds.includes(group.id)}
                        <Check class="w-3 h-3 text-white" />
                      {/if}
                    </div>
                    <span class="text-sm" style="color: var(--ds-text);">{group.name}</span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Organisations -->
          <div>
            <div class="flex items-center gap-2 mb-2">
              <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              <span class="text-sm font-medium" style="color: var(--ds-text);">{t('portal.visibility.organizations')}</span>
            </div>
            {#if organisations.length === 0}
              <div class="text-sm p-3 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-surface-raised);">
                {t('portal.visibility.noOrganizationsAvailable')}
              </div>
            {:else}
              <div class="border rounded divide-y max-h-48 overflow-y-auto" style="border-color: var(--ds-border);">
                {#each organisations as org}
                  <button
                    type="button"
                    onclick={() => toggleOrg(org.id)}
                    class="w-full flex items-center gap-3 px-3 py-2 text-left transition-all hover:bg-black/5"
                  >
                    <div
                      class="w-5 h-5 rounded border-2 flex items-center justify-center"
                      style="border-color: {selectedOrgIds.includes(org.id) ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-border)'}; background-color: {selectedOrgIds.includes(org.id) ? 'var(--ds-interactive, #2563eb)' : 'transparent'};"
                    >
                      {#if selectedOrgIds.includes(org.id)}
                        <Check class="w-3 h-3 text-white" />
                      {/if}
                    </div>
                    <span class="text-sm" style="color: var(--ds-text);">{org.name}</span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Action Buttons -->
      <div class="flex justify-end gap-3 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
        <button
          type="button"
          onclick={onClose}
          class="px-4 py-2 rounded text-sm font-medium transition-all"
          style="color: var(--ds-text); background-color: var(--ds-surface-raised);"
          disabled={saving}
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={handleSave}
          class="px-4 py-2 rounded text-sm font-medium text-white transition-all flex items-center gap-2"
          style="background-color: var(--ds-interactive, #2563eb);"
          disabled={saving}
        >
          {#if saving}
            <Spinner size="sm" />
          {/if}
          {t('common.save')}
        </button>
      </div>
    {/if}
  </div>
</Modal>
