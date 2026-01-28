<script>
  import { onMount } from 'svelte';
  import { navigate } from '../router.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { Users, Mail, Search, GripVertical, Plus, Edit2, Trash2, MoreHorizontal } from 'lucide-svelte';
  import { api } from '../api.js';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import Avatar from '../components/Avatar.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Spinner from '../components/Spinner.svelte';
  import CustomerOrganisationNavigation from './CustomerOrganisationNavigation.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import TextField from '../components/TextField.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // State
  let customerOrganisations = $state([]);
  let portalCustomers = $state([]);
  let selectedOrgId = $state(null);
  let loading = $state(true);
  let error = $state(null);

  // Custom fields
  let customFields = $state([]);
  let portalCustomerFields = $state([]);

  // Search filters
  let orgSearch = $state('');
  let customerSearch = $state('');

  // Pagination
  let displayLimit = $state(15);

  // Drag and drop tracking
  let setupElements = new Map();
  let setupTimeout;
  let dragOverOrgId = $state(undefined);


  // Create customer modal
  let showCreateModal = $state(false);
  let formData = $state({
    name: '',
    email: '',
    phone: '',
    customer_organisation_id: null,
    custom_field_values: {}
  });

  // Detail/edit modal
  let showDetailModal = $state(false);
  let selectedCustomer = $state(null);
  let editFormData = $state({
    name: '',
    email: '',
    phone: '',
    customer_organisation_id: null,
    custom_field_values: {}
  });

  // Derived state
  let filteredOrganisations = $derived(
    customerOrganisations.filter(org =>
      org.name.toLowerCase().includes(orgSearch.toLowerCase())
    )
  );

  let unassignedCount = $derived(
    portalCustomers.filter(c => !c.customer_organisation_id).length
  );

  let filteredCustomers = $derived(
    portalCustomers
      .filter(c => {
        if (selectedOrgId === null) {
          return !c.customer_organisation_id;
        }
        return c.customer_organisation_id === selectedOrgId;
      })
      .filter(c => {
        if (!customerSearch) return true;
        const search = customerSearch.toLowerCase();
        return c.name.toLowerCase().includes(search) ||
               c.email.toLowerCase().includes(search);
      })
  );

  let displayedCustomers = $derived(filteredCustomers.slice(0, displayLimit));
  let hasMoreCustomers = $derived(filteredCustomers.length > displayLimit);

  let customerCounts = $derived(
    customerOrganisations.reduce((acc, org) => {
      acc[org.id] = portalCustomers.filter(c => c.customer_organisation_id === org.id).length;
      return acc;
    }, {})
  );

  // Reset pagination when org changes
  $effect(() => {
    selectedOrgId;
    displayLimit = 15;
  });

  // Setup drag and drop after rendering (track both customers and orgs)
  $effect(() => {
    // Track dependencies
    const _customers = displayedCustomers;
    const _orgs = filteredOrganisations;

    if (typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => {
        setupDragAndDrop();
      }, 100);
    }
  });

  onMount(async () => {
    await Promise.all([loadOrganisations(), loadPortalCustomers(), loadCustomFields()]);
    loading = false;
  });

  $effect(() => {
    return () => {
      if (setupTimeout) {
        clearTimeout(setupTimeout);
      }
      setupElements.forEach(cleanup => cleanup());
      setupElements.clear();
    };
  });

  async function loadCustomFields() {
    try {
      customFields = await api.customFields.getAll();
      portalCustomerFields = customFields.filter(f => f.applies_to_portal_customers);
    } catch (err) {
      console.error('Failed to load custom fields:', err);
    }
  }

  async function loadOrganisations() {
    try {
      const result = await api.time.customers.getAll();
      customerOrganisations = result || [];
    } catch (err) {
      console.error('Failed to load customer organisations:', err);
      error = t('workspaces.customers.failedToLoadOrganisations');
    }
  }

  async function loadPortalCustomers() {
    try {
      portalCustomers = await api.portalCustomers.getAll();
    } catch (err) {
      console.error('Failed to load portal customers:', err);
      error = t('workspaces.customers.failedToLoadCustomers');
    }
  }

  function selectOrganisation(orgId) {
    selectedOrgId = orgId;
    setupElements.forEach(cleanup => cleanup());
    setupElements.clear();
  }

  function loadMoreCustomers() {
    displayLimit += 15;
  }

  async function handleCustomerDrop(customerId, targetOrgId) {
    try {
      await api.portalCustomers.updateOrganisation(customerId, targetOrgId);
      await loadPortalCustomers();
    } catch (err) {
      console.error('Failed to update customer organisation:', err);
      errorToast(t('workspaces.customers.failedToAssignCustomer'));
    }
  }

  function setupDragAndDrop() {
    if (setupTimeout) {
      clearTimeout(setupTimeout);
    }

    setupElements.forEach((cleanup, elementId) => {
      if (typeof cleanup === 'function') {
        cleanup();
      }
    });
    setupElements.clear();

    // Setup customers as draggable
    const customerElements = document.querySelectorAll('[data-customer-id]');
    customerElements.forEach(element => {
      const customerId = parseInt(element.dataset.customerId);
      const elementId = `customer-${customerId}`;

      const dragHandle = element.querySelector('[data-drag-handle]');
      if (!dragHandle) return;

      const draggableCleanup = draggable({
        element: element,
        dragHandle: dragHandle,
        getInitialData: () => ({ customerId, type: 'portal-customer' }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          document.body.classList.add('is-dragging');
        },
        onDrop: () => {
          element.style.opacity = '';
          document.body.classList.remove('is-dragging');
        }
      });

      setupElements.set(elementId, () => {
        draggableCleanup();
      });
    });

    // Setup organisation items as drop targets
    const orgElements = document.querySelectorAll('[data-org-id]');
    orgElements.forEach(element => {
      const orgIdStr = element.dataset.orgId;
      const orgId = orgIdStr === 'null' ? null : parseInt(orgIdStr);
      const elementId = `org-${orgIdStr}`;

      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          return data.type === 'portal-customer' && data.customerId !== undefined;
        },
        onDragEnter: () => {
          dragOverOrgId = orgId;
        },
        onDragLeave: () => {
          dragOverOrgId = undefined;
        },
        onDrop: ({ source }) => {
          dragOverOrgId = undefined;
          const customerId = source.data.customerId;
          handleCustomerDrop(customerId, orgId);
        }
      });

      setupElements.set(elementId, () => {
        dropTargetCleanup();
      });
    });
  }

  function startCreate() {
    showCreateModal = true;
    resetForm();
  }

  function resetForm() {
    formData = {
      name: '',
      email: '',
      phone: '',
      customer_organisation_id: selectedOrgId !== null ? selectedOrgId : null,
      custom_field_values: {}
    };
  }

  function closeModal() {
    showCreateModal = false;
    resetForm();
  }

  function openDetailModal(customer) {
    selectedCustomer = customer;
    editFormData = {
      name: customer.name,
      email: customer.email,
      phone: customer.phone || '',
      customer_organisation_id: customer.customer_organisation_id ?? null,
      custom_field_values: customer.custom_field_values || {}
    };
    showDetailModal = true;
  }

  function closeDetailModal() {
    showDetailModal = false;
    selectedCustomer = null;
    editFormData = {
      name: '',
      email: '',
      phone: '',
      customer_organisation_id: null,
      custom_field_values: {}
    };
  }

  async function handleCreateCustomer() {
    try {
      await api.portalCustomers.create(formData);
      await loadPortalCustomers();
      closeModal();
    } catch (err) {
      console.error('Failed to create portal customer:', err);
      errorToast(err.message || String(err));
    }
  }

  async function handleUpdateCustomer() {
    try {
      await api.portalCustomers.update(selectedCustomer.id, editFormData);
      await loadPortalCustomers();
      closeDetailModal();
    } catch (err) {
      console.error('Failed to update portal customer:', err);
      errorToast(err.message || String(err));
    }
  }

  async function handleDeleteCustomer(customer) {
    const confirmed = await confirm({
      title: t('workspaces.customers.deleteCustomer'),
      message: t('workspaces.customers.confirmDeleteCustomer', { name: customer.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) {
      return;
    }

    try {
      await api.portalCustomers.delete(customer.id);
      await loadPortalCustomers();
    } catch (err) {
      console.error('Failed to delete portal customer:', err);
      errorToast(err.message || String(err));
    }
  }

  function buildCustomerActions(customer) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit2,
        title: t('common.edit'),
        onClick: () => openDetailModal(customer)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50 hover:text-red-700',
        onClick: () => handleDeleteCustomer(customer)
      }
    ];
  }

  </script>

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Sidebar Navigation -->
  <CustomerOrganisationNavigation
    organisations={filteredOrganisations}
    {selectedOrgId}
    {unassignedCount}
    bind:searchQuery={orgSearch}
    {customerCounts}
    {dragOverOrgId}
    onSelect={selectOrganisation}
    onManageOrgs={() => navigate('/time/customers')}
  />

  <!-- Main Content -->
  <div class="flex-1 p-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner />
      </div>
    {:else if error}
      <div class="bg-red-50 border border-red-200 rounded p-4">
        <p class="text-red-800">{error}</p>
      </div>
    {:else}
      <!-- Header -->
      <div class="flex justify-between items-start mb-6">
        <div class="flex items-center gap-4">
          {#if selectedOrgId !== null}
            {@const selectedOrg = customerOrganisations.find(o => o.id === selectedOrgId)}
            {#if selectedOrg}
              <Avatar
                src={selectedOrg.avatar_url}
                name={selectedOrg.name}
                size="lg"
                variant="blue"
                rounded="md"
              />
            {/if}
          {/if}
          <div>
            <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
              {#if selectedOrgId === null}
                {t('workspaces.customers.unassignedCustomers')}
              {:else}
                {customerOrganisations.find(o => o.id === selectedOrgId)?.name || t('workspaces.customers.title')}
              {/if}
            </h1>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('workspaces.customers.customerCount', { count: filteredCustomers.length })}
            </p>
          </div>
        </div>
        <Button
          variant="primary"
          icon={Plus}
          onclick={startCreate}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('customers', 'add'), guard: () => !showCreateModal }}
        >
          {t('workspaces.customers.addCustomer')}
        </Button>
      </div>

      <!-- Customer Search -->
      <div class="mb-4">
        <div class="relative max-w-md">
          <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
          <input
            type="text"
            bind:value={customerSearch}
            placeholder={t('workspaces.customers.searchCustomers')}
            class="w-full pl-10 pr-4 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />
        </div>
      </div>

      <!-- Customer List -->
      <div class="rounded shadow-sm border overflow-hidden" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        {#if displayedCustomers.length === 0}
          <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
            <Users class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>{t('workspaces.customers.noCustomersFound')}</p>
            {#if customerSearch}
              <p class="text-sm mt-1">{t('workspaces.customers.tryAdjustingSearch')}</p>
            {:else if selectedOrgId === null}
              <p class="text-sm mt-1">{t('workspaces.customers.allCustomersAssigned')}</p>
            {:else}
              <p class="text-sm mt-1">{t('workspaces.customers.dragCustomersHere')}</p>
            {/if}
          </div>
        {:else}
          <div class="divide-y" style="border-color: var(--ds-border);">
            {#each displayedCustomers as customer (customer.id)}
              <div
                data-customer-id={customer.id}
                class="p-4 hover:bg-opacity-50 transition-colors"
                style="background-color: transparent;"
              >
                <div class="flex items-start gap-3">
                  <!-- Drag Handle -->
                  <div data-drag-handle class="cursor-grab active:cursor-grabbing pt-1">
                    <GripVertical class="w-5 h-5" style="color: var(--ds-text-subtle);" />
                  </div>

                  <div class="flex-1 min-w-0">
                    <button
                      onclick={() => openDetailModal(customer)}
                      class="font-medium truncate hover:underline text-left w-full"
                      style="color: var(--ds-text);"
                    >
                      {customer.name}
                    </button>
                    <div class="flex items-center gap-2 mt-1">
                      <Mail class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                      <span class="text-sm truncate" style="color: var(--ds-text-subtle);">
                        {customer.email}
                      </span>
                    </div>
                    {#if customer.user_name}
                      <div class="flex items-center gap-2 mt-1">
                        <Users class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                        <span class="text-sm truncate" style="color: var(--ds-text-subtle);">
                          {t('workspaces.customers.linked')}: {customer.user_name}
                        </span>
                      </div>
                    {/if}
                  </div>

                  <!-- Action Menu -->
                  <DropdownMenu
                    triggerText=""
                    triggerIcon={MoreHorizontal}
                    triggerClass="p-2 rounded hover-bg transition-colors"
                    items={buildCustomerActions(customer)}
                    align="right"
                  />
                </div>
              </div>
            {/each}
          </div>

          <!-- Load More Button -->
          {#if hasMoreCustomers}
            <div class="p-4 border-t text-center" style="border-color: var(--ds-border);">
              <Button variant="default" onclick={loadMoreCustomers}>
                {t('workspaces.customers.loadMore', { count: filteredCustomers.length - displayLimit })}
              </Button>
            </div>
          {/if}
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Create Customer Modal -->
<Modal
  isOpen={showCreateModal}
  maxWidth="max-w-md"
  onSubmit={handleCreateCustomer}
  submitDisabled={!formData.name.trim() || !formData.email.trim()}
  onclose={closeModal}
>
  {#snippet children({ submitHint })}
    <ModalHeader title={t('workspaces.customers.addPortalCustomer')} onClose={closeModal} />

    <div class="p-6 space-y-4">
      <TextField
        label={t('workspaces.customers.fields.name')}
        id="customer-name"
        bind:value={formData.name}
        placeholder={t('workspaces.customers.placeholders.name')}
        required
      />

      <TextField
        label={t('workspaces.customers.fields.email')}
        id="customer-email"
        type="email"
        bind:value={formData.email}
        placeholder={t('workspaces.customers.placeholders.email')}
        required
      />

      <TextField
        label={t('workspaces.customers.fields.phone')}
        id="customer-phone"
        type="tel"
        bind:value={formData.phone}
        placeholder={t('workspaces.customers.placeholders.phone')}
      />

      <div>
        <Label for="customer-org" class="mb-2">{t('workspaces.customers.fields.customerOrganisation')}</Label>
        <BasePicker
          bind:value={formData.customer_organisation_id}
          items={customerOrganisations}
          placeholder={t('workspaces.customers.noneUnassigned')}
          showUnassigned={true}
          unassignedLabel={t('workspaces.customers.noneUnassigned')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>

      <!-- Custom Fields -->
      {#if portalCustomerFields.length > 0}
        <div class="col-span-full pt-4 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('workspaces.customers.customFields')}</h3>
          <div class="space-y-4">
            {#each portalCustomerFields as field}
              <CustomFieldRenderer
                {field}
                bind:value={formData.custom_field_values[field.name]}
                readonly={false}
                onChange={(val) => {
                  formData.custom_field_values[field.name] = val;
                }}
              />
            {/each}
          </div>
        </div>
      {/if}
    </div>

    <DialogFooter
      onCancel={closeModal}
      onConfirm={handleCreateCustomer}
      confirmLabel={t('workspaces.customers.createCustomer')}
      disabled={!formData.name.trim() || !formData.email.trim()}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>

<!-- Detail/Edit Customer Modal -->
<Modal
  isOpen={showDetailModal && selectedCustomer !== null}
  maxWidth="max-w-2xl"
  onSubmit={handleUpdateCustomer}
  submitDisabled={!editFormData.name.trim() || !editFormData.email.trim()}
  onclose={closeDetailModal}
>
  {#snippet children({ submitHint })}
    <ModalHeader title={t('workspaces.customers.editPortalCustomer')} icon={Edit2} onClose={closeDetailModal} />

    <div class="p-6 space-y-4">
      <TextField
        label={t('workspaces.customers.fields.name')}
        id="edit-customer-name"
        bind:value={editFormData.name}
        placeholder={t('workspaces.customers.placeholders.name')}
        required
      />

      <TextField
        label={t('workspaces.customers.fields.email')}
        id="edit-customer-email"
        type="email"
        bind:value={editFormData.email}
        placeholder={t('workspaces.customers.placeholders.email')}
        required
      />

      <TextField
        label={t('workspaces.customers.fields.phone')}
        id="edit-customer-phone"
        type="tel"
        bind:value={editFormData.phone}
        placeholder={t('workspaces.customers.placeholders.phone')}
      />

      <div>
        <Label for="edit-customer-org" class="mb-2">{t('workspaces.customers.fields.customerOrganisation')}</Label>
        <BasePicker
          bind:value={editFormData.customer_organisation_id}
          items={customerOrganisations}
          placeholder={t('workspaces.customers.noneUnassigned')}
          showUnassigned={true}
          unassignedLabel={t('workspaces.customers.noneUnassigned')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>

      <!-- Custom Fields -->
      {#if portalCustomerFields.length > 0}
        <div class="pt-4 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('workspaces.customers.customFields')}</h3>
          <div class="space-y-4">
            {#each portalCustomerFields as field}
              <CustomFieldRenderer
                {field}
                bind:value={editFormData.custom_field_values[field.name]}
                readonly={false}
                onChange={(val) => {
                  editFormData.custom_field_values[field.name] = val;
                }}
              />
            {/each}
          </div>
        </div>
      {/if}

      <!-- Metadata -->
      {#if selectedCustomer?.created_at}
        <div class="pt-4 border-t space-y-2" style="border-color: var(--ds-border);">
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            <span class="font-medium">{t('workspaces.customers.metadata.created')}:</span> {new Date(selectedCustomer.created_at).toLocaleString()}
          </div>
          {#if selectedCustomer.updated_at}
            <div class="text-xs" style="color: var(--ds-text-subtle);">
              <span class="font-medium">{t('workspaces.customers.metadata.updated')}:</span> {new Date(selectedCustomer.updated_at).toLocaleString()}
            </div>
          {/if}
          {#if selectedCustomer.user_name}
            <div class="text-xs" style="color: var(--ds-text-subtle);">
              <span class="font-medium">{t('workspaces.customers.metadata.linkedUser')}:</span> {selectedCustomer.user_name} ({selectedCustomer.user_email})
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <DialogFooter
      onCancel={closeDetailModal}
      onConfirm={handleUpdateCustomer}
      confirmLabel={t('common.saveChanges')}
      disabled={!editFormData.name.trim() || !editFormData.email.trim()}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>
