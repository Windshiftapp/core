<script>
  import { onMount } from 'svelte';
  import { IconArrowLeft as ArrowLeft, IconUsers as Users, IconMail as Mail, IconPhone as Phone, IconEdit as Edit2, IconSend as Send, IconMessage as MessageCircle } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import Avatar from '../components/Avatar.svelte';
  import Tabs from '../components/Tabs.svelte';
  import Spinner from '../components/Spinner.svelte';
  import TextField from '../components/TextField.svelte';
  import Label from '../components/Label.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    contactId,
    customerOrganisations = [],
    portalCustomerFields = [],
    onBack = () => {},
    onCustomerUpdated = () => {},
  } = $props();

  // State
  let customer = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let isEditing = $state(false);
  let saving = $state(false);

  let editFormData = $state({
    name: '',
    email: '',
    phone: '',
    customer_organisation_id: null,
    custom_field_values: {}
  });

  // Tabs
  let activeTab = $state('overview');
  const tabs = [
    { id: 'overview', label: t('common.overview') || 'Overview', icon: Users },
    { id: 'submissions', label: t('workspaces.customers.submissions') || 'Submissions', icon: Send },
    { id: 'channels', label: t('workspaces.customers.channels') || 'Channels', icon: MessageCircle },
  ];

  // Lazy-loaded data
  let submissions = $state(null);
  let channels = $state(null);
  let loadingSubmissions = $state(false);
  let loadingChannels = $state(false);

  let orgName = $derived(
    customer?.customer_organisation_id
      ? customerOrganisations.find(o => o.id === customer.customer_organisation_id)?.name
      : null
  );

  onMount(async () => {
    await loadCustomer();
  });

  async function loadCustomer() {
    loading = true;
    error = null;
    try {
      customer = await api.portalCustomers.getById(contactId);
    } catch (err) {
      console.error('Failed to load customer:', err);
      error = t('workspaces.customers.failedToLoadCustomer') || 'Failed to load customer';
    } finally {
      loading = false;
    }
  }

  function startEditing() {
    editFormData = {
      name: customer.name,
      email: customer.email,
      phone: customer.phone || '',
      customer_organisation_id: customer.customer_organisation_id ?? null,
      custom_field_values: customer.custom_field_values || {}
    };
    isEditing = true;
  }

  function cancelEditing() {
    isEditing = false;
  }

  async function saveChanges() {
    saving = true;
    try {
      await api.portalCustomers.update(customer.id, editFormData);
      customer = await api.portalCustomers.getById(contactId);
      isEditing = false;
      onCustomerUpdated();
    } catch (err) {
      console.error('Failed to update customer:', err);
      errorToast(err.message || String(err));
    } finally {
      saving = false;
    }
  }

  async function loadSubmissions() {
    if (submissions !== null) return;
    loadingSubmissions = true;
    try {
      submissions = await api.portalCustomers.getSubmissions(contactId);
    } catch (err) {
      console.error('Failed to load submissions:', err);
      submissions = [];
    } finally {
      loadingSubmissions = false;
    }
  }

  async function loadChannels() {
    if (channels !== null) return;
    loadingChannels = true;
    try {
      channels = await api.portalCustomers.getChannels(contactId);
    } catch (err) {
      console.error('Failed to load channels:', err);
      channels = [];
    } finally {
      loadingChannels = false;
    }
  }

  // Lazy-load data when switching tabs
  $effect(() => {
    if (activeTab === 'submissions') {
      loadSubmissions();
    } else if (activeTab === 'channels') {
      loadChannels();
    }
  });
</script>

<div>
  <!-- Back button -->
  <button
    onclick={onBack}
    class="flex items-center gap-2 mb-4 text-sm hover:underline"
    style="color: var(--ds-text-subtle);"
  >
    <ArrowLeft class="w-4 h-4" />
    {t('workspaces.customers.backToCustomers') || 'Back to Customers'}
  </button>

  {#if loading}
    <div class="flex items-center justify-center h-64">
      <Spinner />
    </div>
  {:else if error}
    <div class="bg-red-50 border border-red-200 rounded p-4">
      <p class="text-red-800">{error}</p>
    </div>
  {:else if customer}
    <!-- Header Section -->
    <div class="flex items-center gap-4 mb-6">
      <Avatar name={customer.name} size="xl" variant="blue" rounded="full" />
      <div class="flex-1 min-w-0">
        <h1 class="text-xl font-semibold truncate" style="color: var(--ds-text);">{customer.name}</h1>
        <div class="flex items-center gap-4 mt-1">
          {#if customer.email}
            <div class="flex items-center gap-1.5">
              <Mail class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
              <span class="text-sm" style="color: var(--ds-text-subtle);">{customer.email}</span>
            </div>
          {/if}
          {#if customer.phone}
            <div class="flex items-center gap-1.5">
              <Phone class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
              <span class="text-sm" style="color: var(--ds-text-subtle);">{customer.phone}</span>
            </div>
          {/if}
        </div>
        {#if orgName}
          <span class="inline-block mt-2 text-xs px-2 py-0.5 rounded-full" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
            {orgName}
          </span>
        {/if}
      </div>
    </div>

    <!-- Tabs -->
    <Tabs {tabs} bind:activeTab>
      {#if activeTab === 'overview'}
        <!-- Edit button -->
        <div class="flex justify-end mb-4">
          {#if !isEditing}
            <Button variant="default" icon={Edit2} onclick={startEditing}>
              {t('common.edit')}
            </Button>
          {/if}
        </div>

        {#if isEditing}
          <!-- Edit Form -->
          <div class="space-y-4">
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

            <!-- Save / Cancel -->
            <div class="flex items-center gap-3 pt-4">
              <Button
                variant="primary"
                onclick={saveChanges}
                disabled={!editFormData.name.trim() || !editFormData.email.trim() || saving}
              >
                {saving ? (t('common.saving') || 'Saving...') : t('common.saveChanges')}
              </Button>
              <Button variant="default" onclick={cancelEditing}>
                {t('common.cancel')}
              </Button>
            </div>
          </div>
        {:else}
          <!-- Read-only details -->
          <div class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('workspaces.customers.fields.name')}</div>
                <div style="color: var(--ds-text);">{customer.name}</div>
              </div>
              <div>
                <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('workspaces.customers.fields.email')}</div>
                <div style="color: var(--ds-text);">{customer.email}</div>
              </div>
              {#if customer.phone}
                <div>
                  <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('workspaces.customers.fields.phone')}</div>
                  <div style="color: var(--ds-text);">{customer.phone}</div>
                </div>
              {/if}
              {#if orgName}
                <div>
                  <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('workspaces.customers.fields.customerOrganisation')}</div>
                  <div style="color: var(--ds-text);">{orgName}</div>
                </div>
              {/if}
            </div>

            <!-- Custom Field Values (read-only) -->
            {#if portalCustomerFields.length > 0 && customer.custom_field_values}
              {@const filledFields = portalCustomerFields.filter(f => customer.custom_field_values[f.name] !== undefined && customer.custom_field_values[f.name] !== null && customer.custom_field_values[f.name] !== '')}
              {#if filledFields.length > 0}
                <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                  <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('workspaces.customers.customFields')}</h3>
                  <div class="grid grid-cols-2 gap-4">
                    {#each filledFields as field}
                      <div>
                        <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{field.label || field.name}</div>
                        <div style="color: var(--ds-text);">{customer.custom_field_values[field.name]}</div>
                      </div>
                    {/each}
                  </div>
                </div>
              {/if}
            {/if}

            <!-- Metadata -->
            {#if customer.created_at}
              <div class="pt-4 border-t space-y-2" style="border-color: var(--ds-border);">
                <div class="text-xs" style="color: var(--ds-text-subtle);">
                  <span class="font-medium">{t('workspaces.customers.metadata.created')}:</span> {new Date(customer.created_at).toLocaleString()}
                </div>
                {#if customer.updated_at}
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    <span class="font-medium">{t('workspaces.customers.metadata.updated')}:</span> {new Date(customer.updated_at).toLocaleString()}
                  </div>
                {/if}
                {#if customer.user_name}
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    <span class="font-medium">{t('workspaces.customers.metadata.linkedUser')}:</span> {customer.user_name} ({customer.user_email})
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/if}

      {:else if activeTab === 'submissions'}
        {#if loadingSubmissions}
          <div class="flex items-center justify-center h-32">
            <Spinner />
          </div>
        {:else if submissions && submissions.length > 0}
          <div class="divide-y" style="border-color: var(--ds-border);">
            {#each submissions as submission (submission.id)}
              <div class="py-3">
                <div class="font-medium text-sm" style="color: var(--ds-text);">{submission.title || submission.subject || `Submission #${submission.id}`}</div>
                {#if submission.created_at}
                  <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{new Date(submission.created_at).toLocaleString()}</div>
                {/if}
              </div>
            {/each}
          </div>
        {:else}
          <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
            <Send class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>{t('workspaces.customers.noSubmissions') || 'No submissions yet'}</p>
          </div>
        {/if}

      {:else if activeTab === 'channels'}
        {#if loadingChannels}
          <div class="flex items-center justify-center h-32">
            <Spinner />
          </div>
        {:else if channels && channels.length > 0}
          <div class="divide-y" style="border-color: var(--ds-border);">
            {#each channels as channel (channel.id)}
              <div class="py-3">
                <div class="font-medium text-sm" style="color: var(--ds-text);">{channel.name || channel.title || `Channel #${channel.id}`}</div>
                {#if channel.type}
                  <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{channel.type}</div>
                {/if}
              </div>
            {/each}
          </div>
        {:else}
          <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
            <MessageCircle class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>{t('workspaces.customers.noChannels') || 'No channels yet'}</p>
          </div>
        {/if}
      {/if}
    </Tabs>
  {/if}
</div>
