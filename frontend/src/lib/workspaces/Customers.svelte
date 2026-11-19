<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import Edit2 from '@tabler/icons-svelte-runes/icons/edit';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import Lock from '@tabler/icons-svelte-runes/icons/lock';
  import Unlock from '@tabler/icons-svelte-runes/icons/lock-open-2';
  import { customFields as customFieldsApi } from '../api/configuration.js';
  import { customerOrganisations as customerOrganisationsApi, portalCustomers as portalCustomersApi } from '../api/portal.js';
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
  import { realCustomers } from '../utils/customerLabel.js';
  import { fieldValue } from '../utils/customFieldValues.js';
  import { permissionStore, isSystemAdmin } from '../stores/permissions.svelte.js';
  import AlertBox from '../components/AlertBox.svelte';

  // Route-derived state
  let contactDetailId = $derived(
    $currentRoute.view === 'organization-contact-detail' ? $currentRoute.params.contactId : null
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

  // Pagination (server-side; the directory can exceed the legacy 500 cap)
  const customerPageSize = 15;
  let customerTotal = $state(0);
  let loadingCustomers = $state(false);
  let customerCounts = $state({});
  let unassignedCount = $state(0);

  let filteredOrganisations = $derived(
    customerOrganisations.filter(org =>
      org.name.toLowerCase().includes(orgSearch.toLowerCase())
    )
  );

  let hasMoreCustomers = $derived(portalCustomers.length < customerTotal);

  const canManage = $derived($permissionStore.userPermissionKeys?.has('customers.manage') || $isSystemAdmin);

  let selectedOrg = $derived(
    selectedOrgId !== null ? customerOrganisations.find(o => o.id === selectedOrgId) : null
  );

  // Reload the first page whenever the selected organisation or search changes.
  let customerQueryKey = null;
  $effect(() => {
    const key = `${selectedOrgId ?? 'unassigned'}\u0000${customerSearch}`;
    if (customerQueryKey === null) {
      customerQueryKey = key;
      return;
    }
    if (key === customerQueryKey) return;
    customerQueryKey = key;
    loadPortalCustomers({ reset: true });
  });

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

  // Setup drag and drop after rendering (track both customers and orgs)
  $effect(() => {
    // Track dependencies
    const _customers = portalCustomers;
    const _orgs = filteredOrganisations;

    if (typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => {
        setupDragAndDrop();
      }, 100);
    }
  });

  onMount(async () => {
    await Promise.all([loadOrganisations(), loadCustomFields()]);
    await loadPortalCustomers({ reset: true });
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
      customFields = await customFieldsApi.getAll();
      portalCustomerFields = customFields.filter(f => f.applies_to_portal_customers);
    } catch (err) {
      console.error('Failed to load custom fields:', err);
    }
  }

  async function loadOrganisations() {
    try {
      const result = await customerOrganisationsApi.getAll();
      customerOrganisations = realCustomers(result);
    } catch (err) {
      console.error('Failed to load customer organisations:', err);
      error = t('workspaces.customers.failedToLoadOrganisations');
    }
  }

  // Replace the stored organisation so a later revisit re-hydrates the detail
  // view from the values that were actually saved.
  function handleOrganisationUpdated(orgId, settings) {
    customerOrganisations = customerOrganisations.map((org) =>
      org.id === orgId ? { ...org, settings } : org
    );
  }

  async function loadPortalCustomers({ reset = false } = {}) {
    loadingCustomers = true;
    try {
      const params = { limit: customerPageSize, offset: reset ? 0 : portalCustomers.length };
      if (customerSearch) params.search = customerSearch;
      if (selectedOrgId === null) {
        params.unassigned = 'true';
      } else {
        params.organisation_id = selectedOrgId;
      }
      const result = await portalCustomersApi.getAll(params);
      if (Array.isArray(result)) {
        // Legacy array contract (no `limit`); keep the previous behavior.
        portalCustomers = reset ? result : [...portalCustomers, ...result];
        customerTotal = portalCustomers.length;
        return;
      }
      const page = result?.customers ?? [];
      portalCustomers = reset ? page : [...portalCustomers, ...page];
      customerTotal = result?.total ?? portalCustomers.length;
      if (result?.counts) {
        customerCounts = result.counts.organisations ?? {};
        unassignedCount = result.counts.unassigned ?? 0;
      }
    } catch (err) {
      console.error('Failed to load portal customers:', err);
      error = t('workspaces.customers.failedToLoadCustomers');
    } finally {
      loadingCustomers = false;
    }
  }

  // Keep the navigation counts in sync after a local move without re-reading
  // the directory.
  function adjustCustomerCount(fromOrgId, toOrgId) {
    const from = fromOrgId ?? 'unassigned';
    const to = toOrgId ?? 'unassigned';
    if (from === to) return;
    if (from === 'unassigned') {
      unassignedCount = Math.max(0, unassignedCount - 1);
    } else {
      customerCounts = { ...customerCounts, [from]: Math.max(0, (customerCounts[from] || 0) - 1) };
    }
    if (to === 'unassigned') {
      unassignedCount += 1;
    } else {
      customerCounts = { ...customerCounts, [to]: (customerCounts[to] || 0) + 1 };
    }
  }

  function selectOrganisation(orgId) {
    selectedOrgId = orgId;
    setupElements.forEach(cleanup => cleanup());
    setupElements.clear();
  }

  function loadMoreCustomers() {
    if (loadingCustomers || !hasMoreCustomers) return;
    loadPortalCustomers({ reset: false });
  }

  async function handleCustomerDrop(customerId, targetOrgId) {
    const customer = portalCustomers.find((c) => c.id === customerId);
    const previousOrgId = customer?.customer_organisation_id ?? null;
    try {
      await portalCustomersApi.updateOrganisation(customerId, targetOrgId);
      // The customer left the current view; drop it locally instead of
      // reloading the whole directory (WI-2122).
      portalCustomers = portalCustomers.filter((c) => c.id !== customerId);
      customerTotal = Math.max(0, customerTotal - 1);
      adjustCustomerCount(previousOrgId, targetOrgId);
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
    const customerElements = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-customer-id]'));
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
    const orgElements = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-org-id]'));
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
    navigate('/organizations/contacts/' + customer.id);
  }

  async function handleCreateCustomer() {
    try {
      await portalCustomersApi.create(formData);
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
      await portalCustomersApi.delete(customer.id);
      await loadPortalCustomers();
    } catch (err) {
      console.error('Failed to delete portal customer:', err);
      errorToast(err.message || String(err));
    }
  }

  // Deactivation cuts portal access (sign-out + auth refusal) without
  // exercising the erasure right; reactivation is the explicit inverse.
  async function handleToggleDeactivation(customer) {
    const deactivating = !customer.deactivated_at;
    const confirmed = await confirm({
      title: deactivating ? t('workspaces.customers.deactivateCustomer') : t('workspaces.customers.activateCustomer'),
      message: deactivating
        ? t('workspaces.customers.confirmDeactivateCustomer', { name: customer.name })
        : t('workspaces.customers.confirmActivateCustomer', { name: customer.name }),
      confirmText: deactivating ? t('workspaces.customers.deactivateCustomer') : t('workspaces.customers.activateCustomer'),
      cancelText: t('common.cancel'),
      variant: deactivating ? 'danger' : 'default'
    });

    if (!confirmed) {
      return;
    }

    try {
      if (deactivating) {
        await portalCustomersApi.deactivate(customer.id);
      } else {
        await portalCustomersApi.activate(customer.id);
      }
      await loadPortalCustomers();
    } catch (err) {
      console.error('Failed to toggle portal customer deactivation:', err);
      errorToast(
        err.message ||
          String(err) ||
          (deactivating ? t('workspaces.customers.failedToDeactivateCustomer') : t('workspaces.customers.failedToActivateCustomer'))
      );
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
      {
        id: 'toggle-deactivation',
        type: 'regular',
        icon: customer.deactivated_at ? Unlock : Lock,
        title: customer.deactivated_at ? t('workspaces.customers.activateCustomer') : t('workspaces.customers.deactivateCustomer'),
        onClick: () => handleToggleDeactivation(customer)
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

<div
  class="flex h-full min-h-0 overflow-hidden"
  style="background-color: var(--ds-surface);"
  data-testid="portal-customers-page"
  data-ready={!loading && !error}
  data-total-customers={customerTotal}
>
  <!-- Sidebar Navigation -->
  <CustomerOrganisationNavigation
    organisations={filteredOrganisations}
    {selectedOrgId}
    {unassignedCount}
    bind:searchQuery={orgSearch}
    {customerCounts}
    {dragOverOrgId}
    onSelect={selectOrganisation}
    onManageOrgs={() => navigate('/time/organizations')}
  />

  <!-- Main Content -->
  <div class="flex-1 min-w-0 min-h-0 overflow-y-auto p-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner />
      </div>
    {:else if error}
      <AlertBox variant="error" message={error} />
    {:else if contactDetailId}
      <ContactDetail
        contactId={contactDetailId}
        {customerOrganisations}
        {portalCustomerFields}
        {canManage}
        onBack={() => navigate('/organizations')}
        onCustomerUpdated={() => loadPortalCustomers({ reset: true })}
      />
    {:else}
      <OrganisationDetail
        organisation={selectedOrg}
        customers={portalCustomers}
        filteredCount={customerTotal}
        loadedCount={portalCustomers.length}
        bind:customerSearch
        {canManage}
        {showCreateModal}
        onStartCreate={startCreate}
        onOpenDetail={openDetail}
        onDeleteCustomer={handleDeleteCustomer}
        {hasMoreCustomers}
        onLoadMore={loadMoreCustomers}
        {buildCustomerActions}
        onOrganisationUpdated={handleOrganisationUpdated}
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
            {#each portalCustomerFields as field (field.id ?? field.name)}
              <CustomFieldRenderer
                {field}
                value={fieldValue(formData.custom_field_values, field) ?? ''}
                readonly={false}
                onChange={(val) => {
                  formData.custom_field_values[String(field.id)] = val;
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
