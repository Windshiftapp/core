<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { IconEdit as Edit2, IconTrash as Trash2 } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Spinner from '../components/Spinner.svelte';
  import CustomerOrganisationNavigation from './CustomerOrganisationNavigation.svelte';
  import OrganisationDetail from './OrganisationDetail.svelte';
  import ContactDetail from './ContactDetail.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import TextField from '../components/TextField.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { permissionStore, isSystemAdmin } from '../stores';

  // Route-derived state
  let contactDetailId = $derived(
    $currentRoute.view === 'customer-contact-detail' ? $currentRoute.params.contactId : null
  );

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

  const canManage = $derived($permissionStore.userPermissionKeys?.has('customers.manage') || $isSystemAdmin);

  let customerCounts = $derived(
    customerOrganisations.reduce((acc, org) => {
      acc[org.id] = portalCustomers.filter(c => c.customer_organisation_id === org.id).length;
      return acc;
    }, {})
  );

  let selectedOrg = $derived(
    selectedOrgId !== null ? customerOrganisations.find(o => o.id === selectedOrgId) : null
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
      customFields = (await api.customFields.getAll())?.data || [];
      portalCustomerFields = customFields.filter(f => f.applies_to_portal_customers);
    } catch (err) {
      console.error('Failed to load custom fields:', err);
    }
  }

  async function loadOrganisations() {
    try {
      const result = await api.customerOrganisations.getAll();
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
    if (!canManage) return;

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

  function openDetail(customer) {
    navigate('/customers/contacts/' + customer.id);
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
    if (!canManage) return [];

    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit2,
        title: t('common.edit'),
        onClick: () => openDetail(customer)
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
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
    {:else if contactDetailId}
      <ContactDetail
        contactId={contactDetailId}
        {customerOrganisations}
        {portalCustomerFields}
        onBack={() => navigate('/customers')}
        onCustomerUpdated={() => loadPortalCustomers()}
      />
    {:else}
      <OrganisationDetail
        organisation={selectedOrg}
        customers={displayedCustomers}
        filteredCount={filteredCustomers.length}
        bind:displayLimit
        bind:customerSearch
        {canManage}
        {showCreateModal}
        onStartCreate={startCreate}
        onOpenDetail={openDetail}
        onDeleteCustomer={handleDeleteCustomer}
        {hasMoreCustomers}
        onLoadMore={loadMoreCustomers}
        {buildCustomerActions}
      />
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
