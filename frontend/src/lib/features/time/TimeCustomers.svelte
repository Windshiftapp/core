<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import TimeCustomerModal from '../../dialogs/TimeCustomerModal.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Text from '../../components/Text.svelte';
  import { createShortcutHandler, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import { Plus, Trash2, Edit, Users } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let customers = [];
  let customFields = [];
  let customerOrgFields = [];
  let showModal = false;
  let editingCustomer = null;
  let formData = {
    name: '',
    email: '',
    description: '',
    active: true,
    avatar_url: null,
    custom_field_values: {}
  };

  // Check if attachments are enabled
  let attachmentsEnabled = false;

  onMount(async () => {
    await Promise.all([loadCustomers(), loadCustomFields(), checkAttachmentsEnabled()]);
  });

  async function checkAttachmentsEnabled() {
    try {
      const settings = await api.attachmentSettings.get();
      attachmentsEnabled = settings?.enabled || false;
    } catch (err) {
      console.error('Failed to check attachments settings:', err);
    }
  }

  async function loadCustomFields() {
    try {
      customFields = await api.customFields.getAll();
      // Filter fields that apply to customer organisations
      customerOrgFields = customFields.filter(f => f.applies_to_customer_organisations);
    } catch (err) {
      console.error('Failed to load custom fields:', err);
    }
  }

  async function loadCustomers() {
    try {
      const result = await api.time.customers.getAll();
      customers = result || [];
    } catch (error) {
      console.error('Failed to load customers:', error);
      customers = []; // Ensure customers is always an array
    }
  }

  function startCreate() {
    showModal = true;
    editingCustomer = null;
    resetForm();
  }

  function startEdit(customer) {
    editingCustomer = customer;
    formData = {
      name: customer.name,
      email: customer.email || '',
      description: customer.description || '',
      active: customer.active,
      avatar_url: customer.avatar_url || null,
      custom_field_values: customer.custom_field_values || {}
    };
    showModal = true;
  }

  function resetForm() {
    formData = {
      name: '',
      email: '',
      description: '',
      active: true,
      avatar_url: null,
      custom_field_values: {}
    };
  }

  function cancelForm() {
    showModal = false;
    editingCustomer = null;
    resetForm();
  }

  async function saveCustomer() {
    try {
      console.log(formData)
      if (editingCustomer) {
        await api.time.customers.update(editingCustomer.id, formData);
      } else {
        await api.time.customers.create(formData);
      }
      await loadCustomers();
      cancelForm();
    } catch (error) {
      console.error('Failed to save customer:', error);
      alert(t('time.organizations.failedToSave') + ': ' + (error.message || error));
    }
  }

  async function deleteCustomer(customer) {
    if (confirm(t('time.organizations.confirmDelete', { name: customer.name }))) {
      try {
        await api.time.customers.delete(customer.id);
        await loadCustomers();
      } catch (error) {
        console.error('Failed to delete customer:', error);
      }
    }
  }

  // Keyboard shortcuts
  const handleKeydown = createShortcutHandler({
    addCustomer: () => {
      if (!showModal) {
        startCreate();
      }
    }
  }, 'timeCustomers');

  // DataTable columns configuration - use $derived for reactivity
  const customerColumns = $derived([
    { key: 'name', label: t('common.name'), slot: 'name' },
    { key: 'email', label: t('common.email'), slot: 'email' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'created_at', label: t('common.created'), render: (customer) => new Date(customer.created_at).toLocaleDateString(), textColor: 'var(--ds-text-subtle)' },
    { key: 'actions', label: t('common.actions') }
  ]);

  // Build dropdown action items for each customer
  function buildCustomerDropdownItems(customer) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(customer)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteCustomer(customer)
      }
    ];
  }
</script>

<!-- Keyboard shortcuts -->
<svelte:window onkeydown={handleKeydown} />

<!-- Header -->
<div class="mb-6 flex justify-between items-start">
  <div>
    <Text as="h2" size="lg" weight="semibold">{t('time.organizations.title')}</Text>
    <Text as="div" size="xs" variant="subtle" class="mt-1">
      {t('time.organizations.subtitle')}
    </Text>
  </div>
    <Button
      variant="primary"
      onclick={startCreate}
      icon={Plus}
      size="medium"
      keyboardHint={getShortcutDisplay('timeCustomers', 'addCustomer')}
    >
      {t('time.organizations.addOrganization')}
    </Button>
  </div>

<!-- Customer Modal -->
<TimeCustomerModal
  isOpen={showModal}
  bind:formData
  {customerOrgFields}
  {attachmentsEnabled}
  isEditing={!!editingCustomer}
  onsave={saveCustomer}
  oncancel={cancelForm}
/>

  <DataTable
    columns={customerColumns}
    data={customers}
    keyField="id"
    emptyMessage={t('time.organizations.noOrganizations')}
    emptyIcon={Users}
    actionItems={buildCustomerDropdownItems}
  >
    <!-- Customer name with avatar/initials and description -->
    <div slot="name" let:item={customer}>
      <div class="flex items-center gap-3">
        <Avatar
          src={customer.avatar_url}
          name={customer.name}
          size="md"
          variant="blue"
          rounded="md"
        />
        <div>
          <Text weight="semibold">{customer.name}</Text>
          {#if customer.description}
            <Text as="div" size="sm" variant="subtle" class="mt-1">{customer.description}</Text>
          {/if}
        </div>
      </div>
    </div>

    <!-- Email -->
    <div slot="email" let:item={customer}>
      <Text size="sm">{customer.email || '—'}</Text>
    </div>

    <!-- Status badge -->
    <div slot="status" let:item={customer}>
      <Lozenge color={customer.active ? 'green' : 'gray'} text={customer.active ? t('common.active') : t('common.inactive')} />
    </div>
  </DataTable>

