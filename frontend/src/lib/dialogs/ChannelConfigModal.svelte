<script>
  import { untrack } from 'svelte';
  import { LifeBuoy, Settings, Webhook, ExternalLink, Users, Globe, Check, X, Plus, Mail } from 'lucide-svelte';
  import { api } from '../api.js';
  import { channelCategoriesStore } from '../stores/channelCategories.js';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Spinner from '../components/Spinner.svelte';
  import ChannelManagersTab from '../settings/ChannelManagersTab.svelte';
  import WorkspacePicker from '../pickers/WorkspacePicker.svelte';
  import CollectionPicker from '../pickers/CollectionPicker.svelte';
  import Label from '../components/Label.svelte';
  import DialogFooter from './DialogFooter.svelte';

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
    // Auth method selection
    auth_method: 'basic',           // 'basic' or 'oauth'

    // Inline OAuth credentials
    oauth_provider_type: 'microsoft',  // 'microsoft' or 'google'
    oauth_client_id: '',
    oauth_client_secret: '',
    oauth_tenant_id: 'common',         // Microsoft only

    // OAuth connection status (read-only, from config)
    oauth_connected: false,
    oauth_email: '',

    // Basic IMAP fields
    imap_host: '',
    imap_port: 993,
    imap_encryption: 'ssl',
    imap_username: '',
    imap_password: '',

    // Common fields
    workspace_id: null,
    item_type_id: null,
    mailbox: 'INBOX',
    mark_as_read: true,
    delete_after_process: false
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

  let webhookTestResult = $state(null);

  // Available webhook events
  const webhookEvents = [
    { id: 'item.created', label: 'Item Created', category: 'Items' },
    { id: 'item.updated', label: 'Item Updated', category: 'Items' },
    { id: 'item.deleted', label: 'Item Deleted', category: 'Items' },
    { id: 'item.assigned', label: 'Item Assigned', category: 'Items' },
    { id: 'status.changed', label: 'Status Changed', category: 'Items' },
    { id: 'comment.created', label: 'Comment Created', category: 'Comments' },
    { id: 'comment.updated', label: 'Comment Updated', category: 'Comments' },
    { id: 'comment.deleted', label: 'Comment Deleted', category: 'Comments' },
    { id: 'item.linked', label: 'Item Linked', category: 'Links' },
    { id: 'item.unlinked', label: 'Item Unlinked', category: 'Links' }
  ];

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
      // Basic info
      channelFormData = {
        name: channel.name || '',
        description: channel.description || '',
        category_id: channel.category_id || null
      };

      // Type-specific config
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
          // Determine auth method from config
          auth_method: config.email_oauth_provider_type ? 'oauth' : 'basic',

          // Inline OAuth credentials
          oauth_provider_type: config.email_oauth_provider_type || 'microsoft',
          oauth_client_id: config.email_oauth_client_id || '',
          oauth_client_secret: '',  // Never loaded from backend (security)
          oauth_tenant_id: config.email_oauth_tenant_id || 'common',

          // OAuth connection status
          oauth_connected: !!config.email_oauth_email,
          oauth_email: config.email_oauth_email || '',

          // Basic IMAP fields
          imap_host: config.imap_host || '',
          imap_port: config.imap_port || 993,
          imap_encryption: config.imap_encryption || 'ssl',
          imap_username: config.imap_username || '',
          imap_password: '',

          // Common fields
          workspace_id: config.email_workspace_id || null,
          item_type_id: config.email_item_type_id || null,
          mailbox: config.email_mailbox || 'INBOX',
          mark_as_read: config.email_mark_as_read !== false,
          delete_after_process: config.email_delete_after_process || false
        };
        loadWorkspacesAndItemTypes();
      }

      activeTab = 'configuration';
      webhookTestResult = null;
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
      'email': Mail
    };
    return icons[type] || LifeBuoy;
  }

  // Validate all required fields based on channel type
  function validateForm() {
    // Basic info - name is required
    if (!channelFormData.name?.trim()) {
      return { valid: false, message: 'Channel name is required' };
    }

    // Portal validation
    if (channel.type === 'portal') {
      if (!portalFormData.slug?.trim()) {
        return { valid: false, message: 'Portal slug is required' };
      }
      if (!portalFormData.workspace_ids?.length) {
        return { valid: false, message: 'Please select at least one target workspace' };
      }
    }

    // Webhook validation
    if (channel.type === 'webhook' && !isPluginOwned(channel)) {
      if (!webhookFormData.url?.trim()) {
        return { valid: false, message: 'Webhook URL is required' };
      }
    }

    // Email validation
    if (channel.type === 'email') {
      if (emailFormData.auth_method === 'basic') {
        if (!emailFormData.imap_host?.trim()) {
          return { valid: false, message: 'IMAP host is required' };
        }
        if (!emailFormData.imap_username?.trim()) {
          return { valid: false, message: 'IMAP username is required' };
        }
      } else if (emailFormData.auth_method === 'oauth') {
        if (!emailFormData.oauth_client_id?.trim()) {
          return { valid: false, message: 'OAuth client ID is required' };
        }
        // Client secret only required if not already connected
        if (!emailFormData.oauth_connected && !emailFormData.oauth_client_secret?.trim()) {
          return { valid: false, message: 'OAuth client secret is required' };
        }
      }

      if (!emailFormData.workspace_id) {
        return { valid: false, message: 'Target workspace is required' };
      }
      if (!emailFormData.item_type_id) {
        return { valid: false, message: 'Item type is required' };
      }
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

      // Save basic info
      await api.channels.update(channel.id, {
        ...channel,
        name: channelFormData.name,
        description: channelFormData.description,
        category_id: channelFormData.category_id
      });

      // Save type-specific config
      if (channel.type === 'portal') {
        const existingConfig = parseChannelConfig(channel.config);
        const configData = {
          ...existingConfig,
          portal_slug: portalFormData.slug,
          portal_workspace_ids: portalFormData.workspace_ids,
          portal_enabled: portalFormData.enabled,
          portal_title: portalFormData.title || portalFormData.slug,
          portal_description: portalFormData.description || ''
        };
        await api.channels.updateConfig(channel.id, configData);
      } else if (channel.type === 'webhook' && !isPluginOwned(channel)) {
        const headersObj = {};
        webhookFormData.headers.forEach(h => {
          if (h.key && h.key.trim()) {
            headersObj[h.key.trim()] = h.value || '';
          }
        });

        const configData = {
          webhook_url: webhookFormData.url,
          webhook_secret: webhookFormData.secret || undefined,
          webhook_headers: Object.keys(headersObj).length > 0 ? headersObj : undefined,
          webhook_scope_type: webhookFormData.scope_type,
          webhook_workspace_ids: webhookFormData.scope_type === 'workspaces' ? webhookFormData.workspace_ids : undefined,
          webhook_collection_ids: webhookFormData.scope_type === 'collections' ? webhookFormData.collection_ids : undefined,
          webhook_auto_trigger: webhookFormData.auto_trigger,
          webhook_subscribed_events: webhookFormData.auto_trigger ? webhookFormData.subscribed_events : undefined
        };
        await api.channels.updateConfig(channel.id, configData);
        webhookFormData.secret = '';
      } else if (channel.type === 'email') {
        const configData = {
          email_auth_method: emailFormData.auth_method,

          // OAuth credentials (only for oauth method)
          ...(emailFormData.auth_method === 'oauth' ? {
            email_oauth_provider_type: emailFormData.oauth_provider_type,
            email_oauth_client_id: emailFormData.oauth_client_id,
            email_oauth_client_secret: emailFormData.oauth_client_secret || undefined,
            email_oauth_tenant_id: emailFormData.oauth_provider_type === 'microsoft'
              ? emailFormData.oauth_tenant_id
              : undefined,
          } : {
            // IMAP credentials (only for basic method)
            imap_host: emailFormData.imap_host,
            imap_port: emailFormData.imap_port,
            imap_encryption: emailFormData.imap_encryption,
            imap_username: emailFormData.imap_username,
            imap_password: emailFormData.imap_password || undefined,
          }),

          // Common fields
          email_workspace_id: emailFormData.workspace_id,
          email_item_type_id: emailFormData.item_type_id,
          email_mailbox: emailFormData.mailbox,
          email_mark_as_read: emailFormData.mark_as_read,
          email_delete_after_process: emailFormData.delete_after_process
        };
        await api.channels.updateConfig(channel.id, configData);
        emailFormData.oauth_client_secret = '';
        emailFormData.imap_password = '';
      }

      toastMessage = 'Channel saved successfully!';
      showToast = true;
      onSave();
    } catch (error) {
      console.error('Failed to save channel:', error);
      toastMessage = 'Failed to save: ' + (error.message || error);
      showToast = true;
    } finally {
      loading = false;
    }
  }

  async function testWebhookSettings() {
    if (!channel || !webhookFormData.url) {
      webhookTestResult = { success: false, message: 'Please enter a webhook URL.' };
      return;
    }

    try {
      new URL(webhookFormData.url);
    } catch {
      webhookTestResult = { success: false, message: 'Please enter a valid URL.' };
      return;
    }

    webhookTestResult = { success: true, message: 'Sending test webhook...', loading: true };

    try {
      const headersObj = {};
      webhookFormData.headers.forEach(h => {
        if (h.key && h.key.trim()) {
          headersObj[h.key.trim()] = h.value || '';
        }
      });

      const configData = {
        webhook_url: webhookFormData.url,
        webhook_secret: webhookFormData.secret || undefined,
        webhook_headers: Object.keys(headersObj).length > 0 ? headersObj : undefined,
        webhook_scope_type: webhookFormData.scope_type,
        webhook_workspace_ids: webhookFormData.scope_type === 'workspaces' ? webhookFormData.workspace_ids : undefined,
        webhook_collection_ids: webhookFormData.scope_type === 'collections' ? webhookFormData.collection_ids : undefined,
        webhook_auto_trigger: webhookFormData.auto_trigger,
        webhook_subscribed_events: webhookFormData.auto_trigger ? webhookFormData.subscribed_events : undefined
      };

      await api.channels.updateConfig(channel.id, configData);

      const result = await api.channels.test(channel.id);
      if (result.success) {
        webhookTestResult = {
          success: true,
          message: 'Test webhook sent successfully! Configuration has been saved.',
          loading: false
        };
        onSave();
      } else {
        webhookTestResult = {
          success: false,
          message: `Webhook test failed: ${result.message || 'Unknown error'}`,
          loading: false
        };
      }
    } catch (error) {
      console.error('Failed to test webhook:', error);
      webhookTestResult = {
        success: false,
        message: 'Webhook test failed: ' + (error.message || error),
        loading: false
      };
    }
  }

  function addWebhookHeader() {
    webhookFormData.headers = [...webhookFormData.headers, { key: '', value: '' }];
  }

  function removeWebhookHeader(index) {
    webhookFormData.headers = webhookFormData.headers.filter((_, i) => i !== index);
  }

  function toggleWebhookEvent(eventId) {
    if (webhookFormData.subscribed_events.includes(eventId)) {
      webhookFormData.subscribed_events = webhookFormData.subscribed_events.filter(e => e !== eventId);
    } else {
      webhookFormData.subscribed_events = [...webhookFormData.subscribed_events, eventId];
    }
  }

  async function startOAuthFlow() {
    if (!channel?.id) return;

    // Validate OAuth credentials
    if (!emailFormData.oauth_client_id) {
      toastMessage = 'Please enter OAuth client ID';
      showToast = true;
      return;
    }

    try {
      loading = true;

      // First save the channel config (including OAuth credentials)
      await handleSaveAll();

      // Start OAuth flow
      const result = await api.channels.startEmailOAuth(channel.id);
      if (result.auth_url) {
        window.location.href = result.auth_url;
      }
    } catch (error) {
      console.error('Failed to start OAuth:', error);
      toastMessage = 'Failed to start OAuth: ' + (error.message || error);
      showToast = true;
    } finally {
      loading = false;
    }
  }

  function handleClose() {
    activeTab = 'configuration';
    webhookTestResult = null;
    showToast = false;
    onClose();
  }
</script>

<Modal
  {isOpen}
  on:close={handleClose}
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
            Open Portal
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
              <span>Configuration</span>
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
                <span>Managers</span>
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
            <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Basic Information</h4>
            <div class="space-y-4">
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <Label color="default" class="mb-2">Name</Label>
                  <Input bind:value={channelFormData.name} placeholder="Channel name" />
                </div>
                <div>
                  <Label color="default" class="mb-2">Category</Label>
                  <Select bind:value={channelFormData.category_id}>
                    <option value={null}>No Category</option>
                    {#each $channelCategoriesStore as category}
                      <option value={category.id}>{category.name}</option>
                    {/each}
                  </Select>
                </div>
              </div>
              <div>
                <Label color="default" class="mb-2">Description</Label>
                <Textarea bind:value={channelFormData.description} rows={2} placeholder="Brief description..." />
              </div>
            </div>
          </div>

          <!-- Type-specific Configuration -->
          {#if channel.type === 'portal'}
            <div class="pt-6 border-t" style="border-color: var(--ds-border);">
              <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Portal Configuration</h4>

              <form onsubmit={(e) => { e.preventDefault(); handlePortalSubmit(); }} class="space-y-4">
                <div>
                  <Label color="default" required class="mb-2">
                    Portal Slug <span class="text-xs font-normal" style="color: var(--ds-text-subtle);">(URL-friendly identifier)</span>
                  </Label>
                  <Input
                    bind:value={portalFormData.slug}
                    required
                    placeholder="support-portal"
                    pattern="[a-z0-9\-]+"
                    title="Only lowercase letters, numbers, and hyphens allowed"
                  />
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                    Portal URL: /portal/{portalFormData.slug || 'your-slug'}
                  </p>
                </div>

                <div>
                  <WorkspacePicker
                    bind:value={portalFormData.workspace_ids}
                    label="Target Workspaces *"
                    placeholder="Search for workspaces..."
                  />
                </div>

                <div>
                  <Label color="default" class="mb-2">Portal Title</Label>
                  <Input bind:value={portalFormData.title} placeholder="Support Portal" />
                </div>

                <div>
                  <Label color="default" class="mb-2">Description</Label>
                  <Textarea bind:value={portalFormData.description} placeholder="Describe what this portal is for..." rows={2} />
                </div>

                <div class="flex items-center gap-3 p-4 rounded" style="background-color: var(--ds-surface-raised);">
                  <input type="checkbox" id="portalEnabled" bind:checked={portalFormData.enabled} class="w-4 h-4 rounded" />
                  <label for="portalEnabled" class="text-sm font-medium cursor-pointer" style="color: var(--ds-text);">
                    Enable Portal (allow public submissions)
                  </label>
                </div>
              </form>
            </div>
          {:else if channel.type === 'webhook'}
            <div class="pt-6 border-t" style="border-color: var(--ds-border);">
              <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Webhook Configuration</h4>

              {#if isPluginOwned(channel)}
                <div class="p-4 rounded-lg border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
                  <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
                    This webhook is managed by the <strong style="color: var(--ds-text);">{channel.plugin_name}</strong> plugin.
                  </p>
                </div>
              {:else}
                <form onsubmit={(e) => { e.preventDefault(); handleWebhookSubmit(); }} class="space-y-6">
                  <div class="space-y-4">
                    <div>
                      <Label color="default" required class="mb-2">Webhook URL</Label>
                      <Input type="url" bind:value={webhookFormData.url} required placeholder="https://your-server.com/webhook" />
                    </div>

                    <div>
                      <Label color="default" class="mb-2">Secret (optional)</Label>
                      <Input type="password" bind:value={webhookFormData.secret} placeholder="Enter secret to update, leave blank to keep existing" />
                      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        Used to sign requests with HMAC-SHA256.
                      </p>
                    </div>

                    <!-- Custom Headers -->
                    <div>
                      <div class="flex items-center justify-between mb-2">
                        <Label color="default">Custom Headers</Label>
                        <Button type="button" variant="ghost" size="small" onclick={addWebhookHeader}>
                          + Add Header
                        </Button>
                      </div>
                      {#if webhookFormData.headers.length > 0}
                        <div class="space-y-2">
                          {#each webhookFormData.headers as header, index}
                            <div class="flex gap-2 items-center">
                              <Input bind:value={header.key} placeholder="Header name" class="flex-1" />
                              <Input bind:value={header.value} placeholder="Header value" class="flex-1" />
                              <Button type="button" variant="ghost" size="small" onclick={() => removeWebhookHeader(index)}>
                                <X class="w-4 h-4" />
                              </Button>
                            </div>
                          {/each}
                        </div>
                      {/if}
                    </div>
                  </div>

                  <!-- Scope Configuration -->
                  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                    <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">Scope</h5>
                    <div class="space-y-3">
                      <label class="flex items-center gap-2 cursor-pointer">
                        <input type="radio" name="webhookScope" value="all" bind:group={webhookFormData.scope_type} class="w-4 h-4" />
                        <span class="text-sm" style="color: var(--ds-text);">All items (instance-wide)</span>
                      </label>
                      <label class="flex items-center gap-2 cursor-pointer">
                        <input type="radio" name="webhookScope" value="workspaces" bind:group={webhookFormData.scope_type} class="w-4 h-4" />
                        <span class="text-sm" style="color: var(--ds-text);">Specific workspaces</span>
                      </label>
                      {#if webhookFormData.scope_type === 'workspaces'}
                        <div class="ml-6">
                          <WorkspacePicker bind:value={webhookFormData.workspace_ids} placeholder="Select workspaces..." />
                        </div>
                      {/if}
                      <label class="flex items-center gap-2 cursor-pointer">
                        <input type="radio" name="webhookScope" value="collections" bind:group={webhookFormData.scope_type} class="w-4 h-4" />
                        <span class="text-sm" style="color: var(--ds-text);">Specific collections</span>
                      </label>
                      {#if webhookFormData.scope_type === 'collections'}
                        <div class="ml-6">
                          <CollectionPicker bind:value={webhookFormData.collection_ids} placeholder="Select collections..." />
                        </div>
                      {/if}
                    </div>
                  </div>

                  <!-- Event Triggers -->
                  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
                    <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">Automatic Triggers</h5>
                    <label class="flex items-center gap-3 p-3 rounded cursor-pointer" style="background-color: var(--ds-surface-raised);">
                      <input type="checkbox" bind:checked={webhookFormData.auto_trigger} class="w-4 h-4 rounded" />
                      <div>
                        <span class="text-sm font-medium" style="color: var(--ds-text);">Enable automatic triggers</span>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">Automatically send webhooks when selected events occur</p>
                      </div>
                    </label>

                    {#if webhookFormData.auto_trigger}
                      <div class="mt-4 space-y-4">
                        {#each ['Items', 'Comments', 'Links'] as category}
                          <div>
                            <h6 class="text-xs font-medium uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">{category}</h6>
                            <div class="grid grid-cols-2 gap-2">
                              {#each webhookEvents.filter(e => e.category === category) as event}
                                <label class="flex items-center gap-2 p-2 rounded cursor-pointer" style="background-color: var(--ds-surface);">
                                  <input
                                    type="checkbox"
                                    checked={webhookFormData.subscribed_events.includes(event.id)}
                                    onchange={() => toggleWebhookEvent(event.id)}
                                    class="w-4 h-4 rounded"
                                  />
                                  <span class="text-sm" style="color: var(--ds-text);">{event.label}</span>
                                </label>
                              {/each}
                            </div>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                </form>

                <!-- Test Webhook Section -->
                <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
                  <h5 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Test Webhook</h5>
                  <div class="flex gap-2 mb-4">
                    <Button onclick={testWebhookSettings} variant="secondary" disabled={!webhookFormData.url || loading}>
                      Send Test Webhook
                    </Button>
                  </div>

                  {#if webhookTestResult}
                    <div
                      class="p-4 rounded text-sm"
                      style="background-color: {webhookTestResult.success ? 'var(--ds-background-success-subtle)' : 'var(--ds-background-danger-subtle)'}; border: 1px solid {webhookTestResult.success ? 'var(--ds-border-success)' : 'var(--ds-border-danger)'}; color: {webhookTestResult.success ? 'var(--ds-text-success)' : 'var(--ds-text-danger)'};"
                    >
                      {#if webhookTestResult.loading}
                        <div class="flex items-center gap-2">
                          <Spinner size="sm" />
                          <span>{webhookTestResult.message}</span>
                        </div>
                      {:else}
                        <div class="flex items-start gap-2">
                          {#if webhookTestResult.success}
                            <Check class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-icon-success);" />
                          {:else}
                            <X class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-icon-danger);" />
                          {/if}
                          <span>{webhookTestResult.message}</span>
                        </div>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {:else if channel.type === 'email'}
            <div class="pt-6 border-t" style="border-color: var(--ds-border);">
              <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">Email Configuration</h4>

              <div class="space-y-6">
                <!-- Authentication Method -->
                <div class="space-y-4">
                  <h5 class="text-sm font-medium" style="color: var(--ds-text);">Authentication Method</h5>

                  <div class="grid grid-cols-2 gap-3">
                    <button
                      type="button"
                      onclick={() => emailFormData.auth_method = 'basic'}
                      class="p-4 rounded border-2 text-left transition-all"
                      style={emailFormData.auth_method === 'basic'
                        ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                        : 'border-color: var(--ds-border);'}
                    >
                      <div class="font-medium" style="color: var(--ds-text);">Basic (IMAP)</div>
                      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        Username and password
                      </div>
                    </button>

                    <button
                      type="button"
                      onclick={() => emailFormData.auth_method = 'oauth'}
                      class="p-4 rounded border-2 text-left transition-all"
                      style={emailFormData.auth_method === 'oauth'
                        ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                        : 'border-color: var(--ds-border);'}
                    >
                      <div class="font-medium" style="color: var(--ds-text);">OAuth</div>
                      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        Microsoft 365 or Google
                      </div>
                    </button>
                  </div>
                </div>

                <!-- OAuth Configuration -->
                {#if emailFormData.auth_method === 'oauth'}
                  <div class="space-y-4 pt-4 border-t" style="border-color: var(--ds-border);">
                    <!-- Provider Type -->
                    <div>
                      <Label color="default" class="mb-2">Provider</Label>
                      <div class="grid grid-cols-2 gap-3">
                        <button
                          type="button"
                          onclick={() => emailFormData.oauth_provider_type = 'microsoft'}
                          class="p-3 rounded border-2 text-left transition-all flex items-center gap-3"
                          style={emailFormData.oauth_provider_type === 'microsoft'
                            ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                            : 'border-color: var(--ds-border);'}
                        >
                          <div class="font-medium" style="color: var(--ds-text);">Microsoft 365</div>
                        </button>
                        <button
                          type="button"
                          onclick={() => emailFormData.oauth_provider_type = 'google'}
                          class="p-3 rounded border-2 text-left transition-all flex items-center gap-3"
                          style={emailFormData.oauth_provider_type === 'google'
                            ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                            : 'border-color: var(--ds-border);'}
                        >
                          <div class="font-medium" style="color: var(--ds-text);">Google</div>
                        </button>
                      </div>
                    </div>

                    <!-- OAuth Credentials -->
                    <div class="grid grid-cols-2 gap-4">
                      <div>
                        <Label color="default" required class="mb-2">Client ID</Label>
                        <Input bind:value={emailFormData.oauth_client_id} placeholder="Application (client) ID" />
                      </div>
                      <div>
                        <Label color="default" required class="mb-2">Client Secret</Label>
                        <Input
                          type="password"
                          bind:value={emailFormData.oauth_client_secret}
                          placeholder={emailFormData.oauth_connected ? 'Leave blank to keep existing' : 'Client secret value'}
                        />
                      </div>
                    </div>

                    {#if emailFormData.oauth_provider_type === 'microsoft'}
                      <div>
                        <Label color="default" class="mb-2">Tenant ID</Label>
                        <Input bind:value={emailFormData.oauth_tenant_id} placeholder="common (multi-tenant) or specific tenant ID" />
                        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                          Use "common" to allow any Microsoft account, or enter a specific tenant ID
                        </p>
                      </div>
                    {/if}

                    <!-- Connection Status -->
                    {#if emailFormData.oauth_connected}
                      <div class="p-4 rounded-lg border" style="background: var(--ds-background-success-subtle); border-color: var(--ds-border-success);">
                        <div class="flex items-center gap-3">
                          <Check class="w-5 h-5" style="color: var(--ds-icon-success);" />
                          <div class="flex-1">
                            <div class="font-medium" style="color: var(--ds-text);">Connected</div>
                            <div class="text-sm" style="color: var(--ds-text-subtle);">
                              {emailFormData.oauth_email}
                            </div>
                          </div>
                          <Button variant="ghost" size="small" onclick={startOAuthFlow} disabled={loading}>
                            Reconnect
                          </Button>
                        </div>
                      </div>
                    {:else if emailFormData.oauth_client_id}
                      <div class="p-4 rounded-lg border" style="background: var(--ds-surface-raised); border-color: var(--ds-border);">
                        <div class="flex items-center justify-between">
                          <div>
                            <div class="font-medium" style="color: var(--ds-text);">Not Connected</div>
                            <div class="text-sm" style="color: var(--ds-text-subtle);">
                              Save settings, then connect your mailbox
                            </div>
                          </div>
                          <Button variant="primary" onclick={startOAuthFlow} disabled={loading}>
                            Connect Mailbox
                          </Button>
                        </div>
                      </div>
                    {/if}

                    <!-- Callback URL Info -->
                    <div class="p-3 rounded border" style="background: var(--ds-surface); border-color: var(--ds-border);">
                      <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Redirect URI (for Azure AD / Google Console)</div>
                      <code class="text-xs" style="color: var(--ds-text);">
                        {window.location.origin}/api/channels/inline-oauth/callback
                      </code>
                    </div>
                  </div>
                {:else}
                  <!-- Basic IMAP Configuration -->
                  <div class="space-y-4 pt-4 border-t" style="border-color: var(--ds-border);">
                    <h5 class="text-sm font-medium" style="color: var(--ds-text);">IMAP Connection</h5>

                    <div class="grid grid-cols-2 gap-4">
                      <div>
                        <Label color="default" required class="mb-2">IMAP Host</Label>
                        <Input bind:value={emailFormData.imap_host} placeholder="imap.example.com" />
                      </div>
                      <div class="grid grid-cols-2 gap-4">
                        <div>
                          <Label color="default" class="mb-2">Port</Label>
                          <Input type="number" bind:value={emailFormData.imap_port} placeholder="993" />
                        </div>
                        <div>
                          <Label color="default" class="mb-2">Encryption</Label>
                          <Select bind:value={emailFormData.imap_encryption}>
                            <option value="ssl">SSL</option>
                            <option value="tls">TLS (STARTTLS)</option>
                            <option value="none">None</option>
                          </Select>
                        </div>
                      </div>
                    </div>

                    <div class="grid grid-cols-2 gap-4">
                      <div>
                        <Label color="default" required class="mb-2">Username</Label>
                        <Input bind:value={emailFormData.imap_username} placeholder="user@example.com" />
                      </div>
                      <div>
                        <Label color="default" required class="mb-2">Password</Label>
                        <Input type="password" bind:value={emailFormData.imap_password} placeholder="Enter password to update" />
                        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Leave blank to keep existing password</p>
                      </div>
                    </div>
                  </div>
                {/if}

                <!-- Item Creation -->
                <div class="pt-4 border-t space-y-4" style="border-color: var(--ds-border);">
                  <h5 class="text-sm font-medium" style="color: var(--ds-text);">Item Creation</h5>

                  <div class="grid grid-cols-2 gap-4">
                    <div>
                      <Label color="default" required class="mb-2">Target Workspace</Label>
                      <Select
                        bind:value={emailFormData.workspace_id}
                        onchange={(e) => {
                          const newWorkspaceId = e.target.value ? parseInt(e.target.value) : null;
                          emailFormData.workspace_id = newWorkspaceId;
                          emailFormData.item_type_id = null;
                          loadItemTypesForWorkspace(newWorkspaceId);
                        }}
                      >
                        <option value={null}>Select workspace...</option>
                        {#each workspaces as ws}
                          <option value={ws.id}>{ws.name}</option>
                        {/each}
                      </Select>
                    </div>
                    <div>
                      <Label color="default" required class="mb-2">Item Type</Label>
                      <Select bind:value={emailFormData.item_type_id} disabled={!emailFormData.workspace_id}>
                        <option value={null}>Select item type...</option>
                        {#each itemTypes as type}
                          <option value={type.id}>{type.name}</option>
                        {/each}
                      </Select>
                      {#if !emailFormData.workspace_id}
                        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Select a workspace first</p>
                      {/if}
                    </div>
                  </div>
                </div>

                <!-- Processing Options -->
                <div class="pt-4 border-t space-y-4" style="border-color: var(--ds-border);">
                  <h5 class="text-sm font-medium" style="color: var(--ds-text);">Processing Options</h5>

                  <div>
                    <Label color="default" class="mb-2">Mailbox</Label>
                    <Input bind:value={emailFormData.mailbox} placeholder="INBOX" />
                    <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">The folder to poll for new emails</p>
                  </div>

                  <div class="space-y-3">
                    <label class="flex items-center gap-3 p-3 rounded cursor-pointer" style="background-color: var(--ds-surface-raised);">
                      <input type="checkbox" bind:checked={emailFormData.mark_as_read} class="w-4 h-4 rounded" />
                      <div>
                        <span class="text-sm font-medium" style="color: var(--ds-text);">Mark as read after processing</span>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">Mark emails as read once they've been converted to items</p>
                      </div>
                    </label>

                    <label class="flex items-center gap-3 p-3 rounded cursor-pointer" style="background-color: var(--ds-surface-raised);">
                      <input type="checkbox" bind:checked={emailFormData.delete_after_process} class="w-4 h-4 rounded" />
                      <div>
                        <span class="text-sm font-medium" style="color: var(--ds-text);">Delete after processing</span>
                        <p class="text-xs" style="color: var(--ds-text-subtle);">Remove emails from the mailbox after creating items (use with caution)</p>
                      </div>
                    </label>
                  </div>
                </div>
              </div>
            </div>
          {:else}
            <div class="pt-6 border-t" style="border-color: var(--ds-border);">
              <div class="text-center py-12">
                <LifeBuoy class="w-16 h-16 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  Configuration for {channel.type} channels coming soon
                </p>
              </div>
            </div>
          {/if}

          <!-- Last Activity -->
          {#if channel.last_activity}
            <div class="pt-6 mt-6 border-t" style="border-color: var(--ds-border);">
              <div class="text-sm" style="color: var(--ds-text-subtle);">
                Last activity: {new Date(channel.last_activity).toLocaleString()}
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
          cancelLabel="Close"
          confirmLabel="Save Changes"
          disabled={loading}
        />
      {:else}
        <DialogFooter
          onCancel={handleClose}
          cancelLabel="Close"
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
