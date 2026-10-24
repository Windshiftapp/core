<script>
  import { onMount } from 'svelte';
  import Badge from '../components/Badge.svelte';
  import Users from '@tabler/icons-svelte-runes/icons/users';
  import Mail from '@tabler/icons-svelte-runes/icons/mail';
  import Phone from '@tabler/icons-svelte-runes/icons/phone';
  import Edit2 from '@tabler/icons-svelte-runes/icons/edit';
  import Send from '@tabler/icons-svelte-runes/icons/send';
  import MessageCircle from '@tabler/icons-svelte-runes/icons/message';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import MoreHorizontal from '@tabler/icons-svelte-runes/icons/dots';
  import { contactRoles as contactRolesApi, portalCustomers } from '../api/portal.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import BackButton from '../components/BackButton.svelte';
  import Avatar from '../components/Avatar.svelte';
  import Tabs from '../components/Tabs.svelte';
  import Spinner from '../components/Spinner.svelte';
  import TextField from '../components/TextField.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { formatAuthenticatedDateTime } from '../utils/authenticatedDateFormatter.js';
  import { itemUrl } from '../utils/urls.js';
  import { getChannelTypeIcon } from '../features/channels/channelTypes.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';

  /**
   * @type {{
   *   contactId?: any,
   *   customerOrganisations?: any[],
   *   portalCustomerFields?: any[],
   *   canManage?: boolean,
   *   onBack?: (e?: any) => void,
   *   onCustomerUpdated?: (...args: any[]) => void,
   * }}
   */
  let {
    contactId,
    customerOrganisations = [],
    portalCustomerFields = [],
    canManage = false,
    onBack = () => {},
    onCustomerUpdated = () => {},
  } = $props();

  // State
  let customer = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let isEditing = $state(false);
  let saving = $state(false);
  let deleting = $state(false);

  let editFormData = $state({
    name: '',
    email: '',
    phone: '',
    customer_organisation_id: null,
    custom_field_values: {},
    role_ids: []
  });

  // Tabs
  let activeTab = $state('overview');
  const tabs = [
    { id: 'overview', label: t('common.overview') || 'Overview', icon: Users, testid: 'customer-detail-overview-tab' },
    { id: 'submissions', label: t('workspaces.customers.submissions') || 'Submissions', icon: Send, testid: 'customer-detail-submissions-tab' },
    { id: 'channels', label: t('workspaces.customers.channels') || 'Channels', icon: MessageCircle, testid: 'customer-detail-channels-tab' },
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

  let contactRoles = $state([]);

  onMount(async () => {
    await loadCustomer();
    try {
      const roles = await contactRolesApi.getAll();
      contactRoles = Array.isArray(roles) ? roles : (roles?.data ?? []);
    } catch (err) {
      console.error('Failed to load contact roles:', err);
    }
  });

  function toggleRole(roleID) {
    editFormData.role_ids = editFormData.role_ids.includes(roleID)
      ? editFormData.role_ids.filter((id) => id !== roleID)
      : [...editFormData.role_ids, roleID];
  }

  async function loadCustomer() {
    loading = true;
    error = null;
    try {
      customer = await portalCustomers.getById(contactId);
    } catch (err) {
      console.error('Failed to load customer:', err);
      error = t('workspaces.customers.failedToLoadCustomer') || 'Failed to load customer';
    } finally {
      loading = false;
    }
  }

  function hasCustomFieldValue(value) {
    if (value === undefined || value === null || value === '') return false;
    return !Array.isArray(value) || value.length > 0;
  }

  function startEditing() {
    editFormData = {
      name: customer.name,
      email: customer.email,
      phone: customer.phone || '',
      customer_organisation_id: customer.customer_organisation_id ?? null,
      custom_field_values: customer.custom_field_values || {},
      role_ids: (customer.roles || []).map((role) => role.id)
    };
    isEditing = true;
  }

  function cancelEditing() {
    isEditing = false;
  }

  async function saveChanges() {
    saving = true;
    try {
      await portalCustomers.update(customer.id, editFormData);
      customer = await portalCustomers.getById(contactId);
      isEditing = false;
      onCustomerUpdated();
    } catch (err) {
      console.error('Failed to update customer:', err);
      errorToast(err.message || String(err));
    } finally {
      saving = false;
    }
  }

  async function deleteCustomer() {
    const confirmed = await confirm({
      title: t('workspaces.customers.deleteCustomer'),
      message: t('workspaces.customers.confirmDeleteCustomer', { name: customer.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) return;

    deleting = true;
    try {
      await portalCustomers.delete(customer.id);
      await onCustomerUpdated();
      onBack();
    } catch (err) {
      console.error('Failed to delete portal customer:', err);
      errorToast(err.message || String(err));
    } finally {
      deleting = false;
    }
  }

  function customerActions() {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit2,
        title: t('common.edit'),
        testid: 'customer-detail-edit',
        onClick: startEditing
      },
      { type: 'divider' },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        testid: 'customer-detail-delete',
        onClick: deleteCustomer
      }
    ];
  }

  async function loadSubmissions() {
    if (submissions !== null) return;
    loadingSubmissions = true;
    try {
      submissions = await portalCustomers.getSubmissions(contactId);
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
      channels = await portalCustomers.getChannels(contactId);
    } catch (err) {
      console.error('Failed to load channels:', err);
      channels = [];
    } finally {
      loadingChannels = false;
    }
  }

  function submissionHref(submission) {
    return itemUrl({ workspaceId: submission.workspace_id, itemId: submission.id });
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
  <BackButton
    onclick={onBack}
    label={t('workspaces.customers.backToCustomers') || 'Back to Customers'}
    class="mb-4"
  />

  {#if loading}
    <div class="flex items-center justify-center h-64">
      <Spinner />
    </div>
  {:else if error}
    <AlertBox variant="error" message={error} />
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
          <Badge size="sm" class="mt-2">{orgName}</Badge>
        {/if}
      </div>
      {#if canManage && !isEditing}
        <DropdownMenu
          triggerIcon={MoreHorizontal}
          triggerClass="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
          triggerStyle="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
          triggerTestid="customer-detail-actions"
          triggerLabel={t('common.actions')}
          items={customerActions()}
          maxWidth="max-w-48"
          placement="bottom-end"
          showChevron={false}
          iconOnly={true}
          disabled={deleting}
        />
      {/if}
    </div>

    <!-- Tabs -->
    <Tabs {tabs} bind:activeTab>
      {#if activeTab === 'overview'}
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

            {#if contactRoles.length > 0}
              <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                <Label class="mb-2">{t('workspaces.customers.contactLevels') || 'Contact levels'}</Label>
                <div class="space-y-2">
                  {#each contactRoles as role (role.id)}
                    <Checkbox
                      checked={editFormData.role_ids.includes(role.id)}
                      onchange={() => toggleRole(role.id)}
                      label={role.name}
                      size="small"
                      dataTestid={`contact-role-${role.id}`}
                    />
                  {/each}
                </div>
              </div>
            {/if}

            <!-- Custom Fields -->
            {#if portalCustomerFields.length > 0}
              <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('workspaces.customers.customFields')}</h3>
                <div class="space-y-4">
                  {#each portalCustomerFields as field}
                    <CustomFieldRenderer
                      {field}
                      value={editFormData.custom_field_values[field.name] ?? ''}
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
              {@const filledFields = portalCustomerFields.filter(f => hasCustomFieldValue(customer.custom_field_values[f.name]))}
              {#if filledFields.length > 0}
                <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                  <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('workspaces.customers.customFields')}</h3>
                  <div class="grid grid-cols-2 gap-4">
                    {#each filledFields as field}
                      <div>
                        <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{field.label || field.name}</div>
                        <CustomFieldRenderer
                          {field}
                          value={customer.custom_field_values[field.name]}
                          readonly={true}
                          noPadding={true}
                          displayTestId={`customer-custom-field-${field.id}-value`}
                        />
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
                  <span class="font-medium">{t('workspaces.customers.metadata.created')}:</span> {formatAuthenticatedDateTime(customer.created_at)}
                </div>
                {#if customer.updated_at}
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    <span class="font-medium">{t('workspaces.customers.metadata.updated')}:</span> {formatAuthenticatedDateTime(customer.updated_at)}
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
              {@const href = submissionHref(submission)}
              <svelte:element
                this={href ? 'a' : 'div'}
                {href}
                class="flex min-w-0 items-start justify-between gap-4 py-3 no-underline {href ? 'group rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2' : ''}"
                style={href ? 'color: inherit; outline-color: var(--ds-border-focused);' : undefined}
                data-testid={`customer-submission-${submission.id}`}
              >
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium {href ? 'group-hover:underline' : ''}" style="color: {href ? 'var(--ds-link)' : 'var(--ds-text)'};">
                    {submission.title || submission.subject || `Submission #${submission.id}`}
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
                    {#if submission.workspace_key && submission.workspace_item_number}
                      <span class="font-medium" style="color: var(--ds-text);">
                        {submission.workspace_key}-{submission.workspace_item_number}
                      </span>
                    {/if}
                    {#if submission.workspace_name}
                      <span>{submission.workspace_name}</span>
                    {/if}
                    {#if submission.status_name}
                      <span
                        class="inline-flex rounded-full px-1.5 py-0.5 font-medium"
                        style="background-color: {submission.status_color}20; color: {submission.status_color};"
                      >
                        {submission.status_name}
                      </span>
                    {/if}
                  </div>
                </div>
                {#if submission.created_at}
                  <DescriptionText as="div" class="shrink-0 text-right">{formatAuthenticatedDateTime(submission.created_at)}</DescriptionText>
                {/if}
              </svelte:element>
            {/each}
          </div>
        {:else}
          <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
            <Send class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>{t('workspaces.customers.noSubmissions') || 'No visible submissions'}</p>
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
              {@const ChannelIcon = getChannelTypeIcon(channel.channel_type)}
              <div class="flex min-w-0 items-start justify-between gap-4 py-3" data-testid={`customer-channel-${channel.channel_id}`}>
                <div class="flex min-w-0 items-start gap-3">
                  <ChannelIcon class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--ds-icon-subtle);" />
                  <div class="min-w-0">
                    <div class="truncate text-sm font-medium" style="color: var(--ds-text);">
                      {channel.channel_name || `Channel #${channel.channel_id}`}
                    </div>
                    <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
                      {#if channel.channel_type}
                        <span class="capitalize">{t(`channels.${channel.channel_type}`, channel.channel_type)}</span>
                      {/if}
                      <span>#{channel.channel_id}</span>
                    </div>
                  </div>
                </div>
                {#if channel.created_at}
                  <DescriptionText as="div" class="shrink-0 text-right">
                    {t('common.created')} {formatAuthenticatedDateTime(channel.created_at)}
                  </DescriptionText>
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
