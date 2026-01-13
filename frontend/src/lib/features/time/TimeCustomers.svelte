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
      if (editingCustomer) {
        await api.time.customers.update(editingCustomer.id, formData);
      } else {
        await api.time.customers.create(formData);
      }
      await loadCustomers();
      cancelForm();
    } catch (error) {
      console.error('Failed to save customer:', error);
      alert('Failed to save customer: ' + (error.message || error));
    }
  }

  async function deleteCustomer(customer) {
    if (confirm(`Are you sure you want to delete "${customer.name}"?`)) {
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

  // DataTable columns configuration
  const customerColumns = [
    { key: 'name', label: 'Name', slot: 'name' },
    { key: 'email', label: 'Email', slot: 'email' },
    { key: 'status', label: 'Status', slot: 'status' },
    { key: 'created_at', label: 'Created', render: (customer) => new Date(customer.created_at).toLocaleDateString(), textColor: 'var(--ds-text-subtle)' },
    { key: 'actions', label: 'Actions' }
  ];

  // Build dropdown action items for each customer
  function buildCustomerDropdownItems(customer) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(customer)
      },
      {
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: 'Delete',
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
    <Text as="h2" size="lg" weight="semibold">Customers</Text>
    <Text as="div" size="xs" variant="subtle" class="mt-1">
      Manage your clients and customer information
    </Text>
  </div>
    <Button
      variant="primary"
      onclick={startCreate}
      icon={Plus}
      size="medium"
      keyboardHint={getShortcutDisplay('timeCustomers', 'addCustomer')}
    >
      Add Customer
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
    emptyMessage="No customers found. Create your first customer to get started."
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
      <Lozenge color={customer.active ? 'green' : 'gray'} text={customer.active ? 'Active' : 'Inactive'} />
    </div>
  </DataTable>
