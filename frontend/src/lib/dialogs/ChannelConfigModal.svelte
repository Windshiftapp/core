<script>
  import { untrack } from 'svelte';
  import { LifeBuoy, Settings, Webhook, ExternalLink, Users, Globe, Mail, Send } from 'lucide-svelte';
  import { api } from '../api.js';
  import { channelCategoriesStore } from '../stores/channelCategories.js';
  import { t } from '../stores/i18n.svelte.js';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import ChannelManagersTab from '../settings/ChannelManagersTab.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from './DialogFooter.svelte';

  // Import channel config components
  import ChannelPortalConfig from '../features/channels/ChannelPortalConfig.svelte';
  import ChannelWebhookConfig from '../features/channels/ChannelWebhookConfig.svelte';
  import ChannelEmailConfig from '../features/channels/ChannelEmailConfig.svelte';
  import ChannelSMTPConfig from '../features/channels/ChannelSMTPConfig.svelte';

  let {
    isOpen = false,
    channel = null,
    onClose = () => {},
    onSave = () => {}
  } = $props();

  let activeTab = $state('configuration');
  let loading = $state(false);

  // Toast state
  let showToast = $state(false);
  let toastMessage = $state('');

  // Config component references
  let portalConfigRef = $state(null);
  let webhookConfigRef = $state(null);
  let emailConfigRef = $state(null);
  let smtpConfigRef = $state(null);

  // Portal configuration form data
  let portalFormData = $state({
    slug: '',
    workspace_ids: [],
    enabled: false,
    title: '',
    description: ''
  });

  // Webhook configuration form data
  let webhookFormData = $state({
    url: '',
    secret: '',
    headers: [],
    scope_type: 'all',
    workspace_ids: [],
    collection_ids: [],
    auto_trigger: false,
    subscribed_events: []
  });

  // Email configuration form data
  let emailFormData = $state({
    auth_method: 'basic',
    oauth_provider_type: 'microsoft',
    oauth_client_id: '',
    oauth_client_secret: '',
    oauth_tenant_id: 'common',
    oauth_connected: false,
    oauth_email: '',
    imap_host: '',
    imap_port: 993,
    imap_encryption: 'ssl',
    imap_username: '',
    imap_password: '',
    workspace_id: null,
    item_type_id: null,
    mailbox: 'INBOX',
    mark_as_read: true,
    delete_after_process: false
  });

  // SMTP configuration form data
  let smtpFormData = $state({
    host: '',
    port: 587,
    username: '',
    password: '',
    from_email: '',
    from_name: '',
    encryption: 'tls'
  });

  // Workspaces and item types for email configuration
  let workspaces = $state([]);
  let itemTypes = $state([]);

  // Channel basic info form
  let channelFormData = $state({
    name: '',
    description: '',
    category_id: null
  });

  // Parse config JSON string
  function parseChannelConfig(config) {
    if (!config) return {};
    if (typeof config === 'string') {
      if (config.trim() === '') return {};
      try {
        return JSON.parse(config);
      } catch (e) {
        console.error('Failed to parse channel config:', e);
        return {};
      }
    }
    return config || {};
  }

  // Initialize form data when channel changes
  $effect(() => {
    if (channel && isOpen) {
      channelFormData = {
        name: channel.name || '',
        description: channel.description || '',
        category_id: channel.category_id || null
      };

      const config = parseChannelConfig(channel.config);

      if (channel.type === 'portal') {
        portalFormData = {
          slug: config.portal_slug || '',
          workspace_ids: config.portal_workspace_ids || [],
          enabled: config.portal_enabled || false,
          title: config.portal_title || '',
          description: config.portal_description || ''
        };
      } else if (channel.type === 'webhook') {
        const headersArray = config.webhook_headers
          ? Object.entries(config.webhook_headers).map(([key, value]) => ({ key, value }))
          : [];

        webhookFormData = {
          url: config.webhook_url || '',
          secret: '',
          headers: headersArray.length > 0 ? headersArray : [],
          scope_type: config.webhook_scope_type || 'all',
          workspace_ids: config.webhook_workspace_ids || [],
          collection_ids: config.webhook_collection_ids || [],
          auto_trigger: config.webhook_auto_trigger || false,
          subscribed_events: config.webhook_subscribed_events || []
        };
      } else if (channel.type === 'email') {
        emailFormData = {
          auth_method: config.email_oauth_provider_type ? 'oauth' : 'basic',
          oauth_provider_type: config.email_oauth_provider_type || 'microsoft',
          oauth_client_id: config.email_oauth_client_id || '',
          oauth_client_secret: '',
          oauth_tenant_id: config.email_oauth_tenant_id || 'common',
          oauth_connected: !!config.email_oauth_email,
          oauth_email: config.email_oauth_email || '',
          imap_host: config.imap_host || '',
          imap_port: config.imap_port || 993,
          imap_encryption: config.imap_encryption || 'ssl',
          imap_username: config.imap_username || '',
          imap_password: '',
          workspace_id: config.email_workspace_id || null,
          item_type_id: config.email_item_type_id || null,
          mailbox: config.email_mailbox || 'INBOX',
          mark_as_read: config.email_mark_as_read !== false,
          delete_after_process: config.email_delete_after_process || false
        };
        loadWorkspacesAndItemTypes();
      } else if (channel.type === 'smtp') {
        smtpFormData = {
          host: config.smtp_host || '',
          port: config.smtp_port || 587,
          username: config.smtp_username || '',
          password: '',
          from_email: config.smtp_from_email || '',
          from_name: config.smtp_from_name || '',
          encryption: config.smtp_encryption || 'tls'
        };
      }

      activeTab = 'configuration';
    }
  });

  async function loadWorkspacesAndItemTypes() {
    try {
      workspaces = await api.workspaces.getAll();
      if (emailFormData.workspace_id) {
        await loadItemTypesForWorkspace(emailFormData.workspace_id);
      }
    } catch (error) {
      console.error('Failed to load workspaces:', error);
    }
  }

  async function loadItemTypesForWorkspace(workspaceId) {
    if (!workspaceId) {
      itemTypes = [];
      return;
    }
    try {
      itemTypes = await api.itemTypes.getForWorkspace(workspaceId);
    } catch (error) {
      console.error('Failed to load item types:', error);
      itemTypes = [];
    }
  }

  function isPluginOwned(ch) {
    return ch?.plugin_name != null;
  }

  function getChannelStatus(ch) {
    return ch?.status || 'disabled';
  }

  function getChannelStatusColor(status) {
    const colors = {
      'enabled': 'green',
      'disabled': 'gray',
      'active': 'green',
      'configured': 'green',
      'pending': 'gray',
      'inactive': 'gray'
    };
    return colors[status] || 'gray';
  }

  function getChannelTypeIcon(type) {
    const icons = {
      'webhook': Webhook,
      'portal': Globe,
      'email': Mail,
      'smtp': Send
    };
    return icons[type] || LifeBuoy;
  }

  // Validate all required fields based on channel type
  function validateForm() {
    if (!channelFormData.name?.trim()) {
      return { valid: false, message: t('channel.channelNameRequired') };
    }

    if (channel.type === 'portal' && portalConfigRef) {
      return portalConfigRef.validate();
    }

    if (channel.type === 'webhook' && !isPluginOwned(channel) && webhookConfigRef) {
      return webhookConfigRef.validate();
    }

    if (channel.type === 'email' && emailConfigRef) {
      return emailConfigRef.validate();
    }

    if (channel.type === 'smtp' && smtpConfigRef) {
      return smtpConfigRef.validate();
    }

    return { valid: true };
  }

  // Unified save function for basic info + type-specific config
  async function handleSaveAll() {
    if (!channel) return;

    const validation = validateForm();
    if (!validation.valid) {
      toastMessage = validation.message;
      showToast = true;
      return;
    }

    try {
      loading = true;

      // Save basic info (excluding config to preserve sensitive fields)
      await api.channels.update(channel.id, {
        id: channel.id,
        type: channel.type,
        direction: channel.direction,
        status: channel.status,
        is_default: channel.is_default,
        name: channelFormData.name,
        description: channelFormData.description,
        category_id: channelFormData.category_id
      });

      // Save type-specific config
      if (channel.type === 'portal' && portalConfigRef) {
        const existingConfig = parseChannelConfig(channel.config);
        const configData = {
          ...existingConfig,
          ...portalConfigRef.getConfig()
        };
        await api.channels.updateConfig(channel.id, configData);
      } else if (channel.type === 'webhook' && !isPluginOwned(channel) && webhookConfigRef) {
        await api.channels.updateConfig(channel.id, webhookConfigRef.getConfig());
        webhookConfigRef.clearSecret?.();
      } else if (channel.type === 'email' && emailConfigRef) {
        await api.channels.updateConfig(channel.id, emailConfigRef.getConfig());
        emailConfigRef.clearSecrets?.();
      } else if (channel.type === 'smtp' && smtpConfigRef) {
        await api.channels.updateConfig(channel.id, smtpConfigRef.getConfig());
        smtpConfigRef.clearSecret?.();
      }

      toastMessage = t('channel.channelSavedSuccess');
      showToast = true;
      onSave();
    } catch (error) {
      console.error('Failed to save channel:', error);
      toastMessage = t('channel.failedToSave') + ': ' + (error.message || error);
      showToast = true;
    } finally {
      loading = false;
    }
  }

  function handleClose() {
    activeTab = 'configuration';
    showToast = false;
    onClose();
  }

  function handleToast(message) {
    toastMessage = message;
    showToast = true;
  }
</script>

<Modal
  {isOpen}
  onclose={handleClose}
  maxWidth="max-w-4xl"
>
  {#if channel}
    <div class="flex flex-col h-[80vh]">
      <!-- Header -->
      <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          <svelte:component this={getChannelTypeIcon(channel.type)} class="w-6 h-6" style="color: var(--ds-text);" />
          <div>
            <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
              {channel.name}
            </h3>
            <div class="flex items-center gap-2 mt-1">
              <span class="text-sm" style="color: var(--ds-text-subtle);">{channel.direction} &bull; {channel.type}</span>
              <Lozenge
                color={getChannelStatusColor(getChannelStatus(channel))}
                text={getChannelStatus(channel)}
              />
              {#if channel.is_default}
                <Lozenge color="blue" text="System" />
              {/if}
              {#if isPluginOwned(channel)}
                <Lozenge color="purple" text="Plugin: {channel.plugin_name}" />
              {/if}
            </div>
          </div>
        </div>
        {#if channel.type === 'portal' && portalFormData.slug}
          <Button
            onclick={() => window.open(`/portal/${portalFormData.slug}`, '_blank')}
            variant="default"
            size="small"
            icon={ExternalLink}
          >
            {t('channel.openPortal')}
          </Button>
        {/if}
      </div>

      <!-- Tab Navigation -->
      <div class="px-6 border-b" style="border-color: var(--ds-border);">
        <nav class="flex gap-6">
          <button
            onclick={() => activeTab = 'configuration'}
            class="relative py-3 text-sm font-medium transition-colors {
              activeTab === 'configuration'
                ? 'text-[var(--ds-interactive)]'
                : 'text-[var(--ds-text-subtle)] hover:text-[var(--ds-text)]'
            }"
          >
            <div class="flex items-center gap-2">
              <Settings class="w-4 h-4" />
              <span>{t('channel.configuration')}</span>
            </div>
            {#if activeTab === 'configuration'}
              <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]"></div>
            {/if}
          </button>

          {#if !isPluginOwned(channel)}
            <button
              onclick={() => activeTab = 'managers'}
              class="relative py-3 text-sm font-medium transition-colors {
                activeTab === 'managers'
                  ? 'text-[var(--ds-interactive)]'
                  : 'text-[var(--ds-text-subtle)] hover:text-[var(--ds-text)]'
              }"
            >
              <div class="flex items-center gap-2">
                <Users class="w-4 h-4" />
                <span>{t('channel.managers')}</span>
              </div>
              {#if activeTab === 'managers'}
                <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]"></div>
              {/if}
            </button>
          {/if}
        </nav>
      </div>

      <!-- Tab Content -->
      <div class="flex-1 overflow-y-auto p-6">
        {#if activeTab === 'configuration'}
          <!-- Basic Info Section -->
          <div class="mb-8">
            <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.basicInformation')}</h4>
            <div class="space-y-4">
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <Label color="default" class="mb-2">{t('channel.name')}</Label>
                  <Input bind:value={channelFormData.name} placeholder={t('channel.channelName')} />
                </div>
                <div>
                  <Label color="default" class="mb-2">{t('channel.category')}</Label>
                  <Select bind:value={channelFormData.category_id}>
                    <option value={null}>{t('channel.noCategory')}</option>
                    {#each $channelCategoriesStore as category}
                      <option value={category.id}>{category.name}</option>
                    {/each}
                  </Select>
                </div>
              </div>
              <div>
                <Label color="default" class="mb-2">{t('channel.description')}</Label>
                <Textarea bind:value={channelFormData.description} rows={2} placeholder={t('channel.briefDescription')} />
              </div>
            </div>
          </div>

          <!-- Type-specific Configuration -->
          {#if channel.type === 'portal'}
            <ChannelPortalConfig
              bind:this={portalConfigRef}
              bind:formData={portalFormData}
            />
          {:else if channel.type === 'webhook'}
            <ChannelWebhookConfig
              bind:this={webhookConfigRef}
              channelId={channel.id}
              bind:formData={webhookFormData}
              isPluginOwned={isPluginOwned(channel)}
              pluginName={channel.plugin_name}
              onSave={onSave}
            />
          {:else if channel.type === 'email'}
            <ChannelEmailConfig
              bind:this={emailConfigRef}
              channelId={channel.id}
              bind:formData={emailFormData}
              {workspaces}
              {itemTypes}
              bind:loading
              onLoadItemTypes={loadItemTypesForWorkspace}
              onToast={handleToast}
            />
          {:else if channel.type === 'smtp'}
            <ChannelSMTPConfig
              bind:this={smtpConfigRef}
              channelId={channel.id}
              bind:formData={smtpFormData}
              onSave={onSave}
            />
          {:else}
            <div class="pt-6 border-t" style="border-color: var(--ds-border);">
              <div class="text-center py-12">
                <LifeBuoy class="w-16 h-16 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {t('channel.comingSoon', { type: channel.type })}
                </p>
              </div>
            </div>
          {/if}

          <!-- Last Activity -->
          {#if channel.last_activity}
            <div class="pt-6 mt-6 border-t" style="border-color: var(--ds-border);">
              <div class="text-sm" style="color: var(--ds-text-subtle);">
                {t('channel.lastActivity')}: {new Date(channel.last_activity).toLocaleString()}
              </div>
            </div>
          {/if}
        {:else if activeTab === 'managers'}
          <ChannelManagersTab
            channelId={channel.id}
            channelName={channel.name}
            isDefault={channel.is_default}
          />
        {/if}
      </div>

      <!-- Footer -->
      {#if activeTab === 'configuration'}
        <DialogFooter
          onCancel={handleClose}
          onConfirm={handleSaveAll}
          cancelLabel={t('common.close')}
          confirmLabel={t('channel.saveChanges')}
          disabled={loading}
        />
      {:else}
        <DialogFooter
          onCancel={handleClose}
          cancelLabel={t('common.close')}
          showCancel={true}
        />
      {/if}
    </div>
  {/if}
</Modal>

<!-- Toast (simple inline for now) -->
{#if showToast}
  <div
    class="fixed bottom-4 right-4 z-50 px-4 py-3 rounded-lg shadow-lg"
    style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
  >
    <p class="text-sm" style="color: var(--ds-text);">{toastMessage}</p>
  </div>
{/if}
