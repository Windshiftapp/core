<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { attachmentStatus } from '../../stores';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import TimeCustomerModal from '../../dialogs/TimeCustomerModal.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Text from '../../components/Text.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { Plus, Trash2, Edit, Users } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import { confirm } from '../../composables/useConfirm.js';

  let customers = $state([]);
  let customFields = $state([]);
  let customerOrgFields = $state([]);
  let showModal = $state(false);
  let editingCustomer = $state(null);
  let formData = $state({
    name: '',
    email: '',
    description: '',
    active: true,
    avatar_url: null,
    custom_field_values: {}
  });

  onMount(async () => {
    await Promise.all([loadCustomers(), loadCustomFields()]);
  });

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
      const result = await api.customerOrganisations.getAll();
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
        await api.customerOrganisations.update(editingCustomer.id, formData);
      } else {
        await api.customerOrganisations.create(formData);
      }
      await loadCustomers();
      cancelForm();
    } catch (error) {
      console.error('Failed to save customer:', error);
      alert(t('time.organizations.failedToSave') + ': ' + (error.message || error));
    }
  }

  async function deleteCustomer(customer) {
    const confirmed = await confirm({
      title: t('time.organizations.deleteOrganization'),
      message: t('time.organizations.confirmDelete', { name: customer.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });

    if (confirmed) {
      try {
        await api.customerOrganisations.delete(customer.id);
        await loadCustomers();
      } catch (error) {
        console.error('Failed to delete customer:', error);
      }
    }
  }

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

<!-- Header -->
<PageHeader
  title={t('time.organizations.title')}
  subtitle={t('time.organizations.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      onclick={startCreate}
      icon={Plus}
      size="medium"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('timeCustomers', 'addCustomer'), guard: () => !showModal }}
    >
      {t('time.organizations.addOrganization')}
    </Button>
  {/snippet}
</PageHeader>

<!-- Customer Modal -->
<TimeCustomerModal
  isOpen={showModal}
  bind:formData
  {customerOrgFields}
  attachmentsEnabled={attachmentStatus.enabled}
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
          <Text size="sm" weight="semibold">{customer.name}</Text>
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

