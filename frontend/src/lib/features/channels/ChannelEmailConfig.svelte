<script>
  import { Check } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import WorkspaceSelector from '../../workspaces/WorkspaceSelector.svelte';

  let {
    channelId,
    formData = $bindable({
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
      delete_after_process: false,
      enabled: false
    }),
    workspaces = [],
    itemTypes = [],
    loading = $bindable(false),
    onLoadItemTypes = () => {},
    onToast = () => {}
  } = $props();

  async function startOAuthFlow() {
    if (!channelId) return;

    if (!formData.oauth_client_id) {
      onToast('Please enter OAuth client ID');
      return;
    }

    try {
      loading = true;
      const result = await api.channels.startEmailOAuth(channelId);
      if (result.auth_url) {
        window.location.href = result.auth_url;
      }
    } catch (error) {
      console.error('Failed to start OAuth:', error);
      onToast('Failed to start OAuth: ' + (error.message || error));
    } finally {
      loading = false;
    }
  }

  export function validate() {
    if (formData.auth_method === 'basic') {
      if (!formData.imap_host?.trim()) {
        return { valid: false, message: t('channel.imapHostRequired') };
      }
      if (!formData.imap_username?.trim()) {
        return { valid: false, message: t('channel.usernameRequired') };
      }
    } else if (formData.auth_method === 'oauth') {
      if (!formData.oauth_client_id?.trim()) {
        return { valid: false, message: t('channel.clientIdRequired') };
      }
      if (!formData.oauth_connected && !formData.oauth_client_secret?.trim()) {
        return { valid: false, message: t('channel.clientSecretRequired') };
      }
    }

    if (!formData.workspace_id) {
      return { valid: false, message: t('channel.targetWorkspaceRequired') };
    }
    if (!formData.item_type_id) {
      return { valid: false, message: t('channel.itemTypeRequired') };
    }

    return { valid: true };
  }

  export function getConfig() {
    const baseConfig = {
      email_auth_method: formData.auth_method,
      email_workspace_id: formData.workspace_id,
      email_item_type_id: formData.item_type_id,
      email_mailbox: formData.mailbox,
      email_mark_as_read: formData.mark_as_read,
      email_delete_after_process: formData.delete_after_process,
      email_enabled: formData.enabled
    };

    if (formData.auth_method === 'oauth') {
      return {
        ...baseConfig,
        email_oauth_provider_type: formData.oauth_provider_type,
        email_oauth_client_id: formData.oauth_client_id,
        email_oauth_client_secret: formData.oauth_client_secret || undefined,
        email_oauth_tenant_id: formData.oauth_provider_type === 'microsoft' ? formData.oauth_tenant_id : undefined
      };
    } else {
      return {
        ...baseConfig,
        imap_host: formData.imap_host,
        imap_port: formData.imap_port,
        imap_encryption: formData.imap_encryption,
        imap_username: formData.imap_username,
        imap_password: formData.imap_password || undefined
      };
    }
  }

  export function clearSecrets() {
    formData.oauth_client_secret = '';
    formData.imap_password = '';
  }
</script>

<div class="pt-6 border-t" style="border-color: var(--ds-border);">
  <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.emailConfiguration')}</h4>

  <div class="space-y-6">
    <!-- Authentication Method -->
    <div class="space-y-4">
      <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.authenticationMethod')}</h5>

      <div class="grid grid-cols-2 gap-3">
        <button
          type="button"
          onclick={() => formData.auth_method = 'basic'}
          class="p-4 rounded border-2 text-left transition-all"
          style={formData.auth_method === 'basic'
            ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
            : 'border-color: var(--ds-border);'}
        >
          <div class="font-medium" style="color: var(--ds-text);">{t('channel.basicIMAP')}</div>
          <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('channel.usernameAndPassword')}
          </div>
        </button>

        <button
          type="button"
          onclick={() => formData.auth_method = 'oauth'}
          class="p-4 rounded border-2 text-left transition-all"
          style={formData.auth_method === 'oauth'
            ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
            : 'border-color: var(--ds-border);'}
        >
          <div class="font-medium" style="color: var(--ds-text);">{t('channel.oauth')}</div>
          <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('channel.microsoftOrGoogle')}
          </div>
        </button>
      </div>
    </div>

    <!-- OAuth Configuration -->
    {#if formData.auth_method === 'oauth'}
      <div class="space-y-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <!-- Provider Type -->
        <div>
          <Label color="default" class="mb-2">{t('channel.provider')}</Label>
          <div class="grid grid-cols-2 gap-3">
            <button
              type="button"
              onclick={() => formData.oauth_provider_type = 'microsoft'}
              class="p-3 rounded border-2 text-left transition-all flex items-center gap-3"
              style={formData.oauth_provider_type === 'microsoft'
                ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                : 'border-color: var(--ds-border);'}
            >
              <div class="font-medium" style="color: var(--ds-text);">{t('channel.microsoft365')}</div>
            </button>
            <button
              type="button"
              onclick={() => formData.oauth_provider_type = 'google'}
              class="p-3 rounded border-2 text-left transition-all flex items-center gap-3"
              style={formData.oauth_provider_type === 'google'
                ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
                : 'border-color: var(--ds-border);'}
            >
              <div class="font-medium" style="color: var(--ds-text);">{t('channel.google')}</div>
            </button>
          </div>
        </div>

        <!-- OAuth Credentials -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <Label color="default" required class="mb-2">{t('channel.clientId')}</Label>
            <Input bind:value={formData.oauth_client_id} placeholder="Application (client) ID" />
          </div>
          <div>
            <Label color="default" required class="mb-2">{t('channel.clientSecret')}</Label>
            <Input
              type="password"
              bind:value={formData.oauth_client_secret}
              placeholder={formData.oauth_connected ? t('channel.leaveBlankToKeep') : 'Client secret value'}
            />
          </div>
        </div>

        {#if formData.oauth_provider_type === 'microsoft'}
          <div>
            <Label color="default" class="mb-2">{t('channel.tenantId')}</Label>
            <Input bind:value={formData.oauth_tenant_id} placeholder="common (multi-tenant) or specific tenant ID" />
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {t('channel.tenantIdHelp')}
            </p>
          </div>
        {/if}

        <!-- Connection Status -->
        {#if formData.oauth_connected}
          <div class="p-4 rounded-lg border" style="background: var(--ds-background-success-subtle); border-color: var(--ds-border-success);">
            <div class="flex items-center gap-3">
              <Check class="w-5 h-5" style="color: var(--ds-icon-success);" />
              <div class="flex-1">
                <div class="font-medium" style="color: var(--ds-text);">{t('channel.connected')}</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  {formData.oauth_email}
                </div>
              </div>
              <Button variant="ghost" size="small" onclick={startOAuthFlow} disabled={loading}>
                {t('channel.reconnect')}
              </Button>
            </div>
          </div>
        {:else if formData.oauth_client_id}
          <div class="p-4 rounded-lg border" style="background: var(--ds-surface-raised); border-color: var(--ds-border);">
            <div class="flex items-center justify-between">
              <div>
                <div class="font-medium" style="color: var(--ds-text);">{t('channel.notConnected')}</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  {t('channel.saveAndConnect')}
                </div>
              </div>
              <Button variant="primary" onclick={startOAuthFlow} disabled={loading}>
                {t('channel.connectMailbox')}
              </Button>
            </div>
          </div>
        {/if}

        <!-- Callback URL Info -->
        <div class="p-3 rounded border" style="background: var(--ds-surface); border-color: var(--ds-border);">
          <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('channel.redirectUri')}</div>
          <code class="text-xs" style="color: var(--ds-text);">
            {typeof window !== 'undefined' ? window.location.origin : ''}/api/channels/inline-oauth/callback
          </code>
        </div>
      </div>
    {:else}
      <!-- Basic IMAP Configuration -->
      <div class="space-y-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.imapConnection')}</h5>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <Label color="default" required class="mb-2">{t('channel.imapHost')}</Label>
            <Input bind:value={formData.imap_host} placeholder="imap.example.com" />
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <Label color="default" class="mb-2">{t('channel.port')}</Label>
              <Input type="number" bind:value={formData.imap_port} placeholder="993" />
            </div>
            <div>
              <Label color="default" class="mb-2">{t('channel.encryption')}</Label>
              <Select bind:value={formData.imap_encryption}>
                <option value="ssl">SSL</option>
                <option value="tls">TLS (STARTTLS)</option>
                <option value="none">None</option>
              </Select>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <Label color="default" required class="mb-2">{t('channel.username')}</Label>
            <Input bind:value={formData.imap_username} placeholder="user@example.com" />
          </div>
          <div>
            <Label color="default" required class="mb-2">{t('channel.password')}</Label>
            <Input type="password" bind:value={formData.imap_password} placeholder="Enter password to update" />
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('channel.leaveBlankPassword')}</p>
          </div>
        </div>
      </div>
    {/if}

    <!-- Item Creation -->
    <div class="pt-4 border-t space-y-4" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.itemCreation')}</h5>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" required class="mb-2">{t('channel.targetWorkspace')}</Label>
          <WorkspaceSelector
            bind:value={formData.workspace_id}
            {workspaces}
            placeholder={t('channel.selectWorkspace')}
            onSelect={(workspace) => {
              formData.item_type_id = null;
              onLoadItemTypes(formData.workspace_id);
            }}
          />
        </div>
        <div>
          <Label color="default" required class="mb-2">{t('channel.itemType')}</Label>
          <Select bind:value={formData.item_type_id} disabled={!formData.workspace_id}>
            <option value={null}>{t('channel.selectItemType')}</option>
            {#each itemTypes as type}
              <option value={type.id}>{type.name}</option>
            {/each}
          </Select>
          {#if !formData.workspace_id}
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('channel.selectWorkspaceFirst')}</p>
          {/if}
        </div>
      </div>
    </div>

    <!-- Processing Options -->
    <div class="pt-4 border-t space-y-4" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.processingOptions')}</h5>

      <div>
        <Label color="default" class="mb-2">{t('channel.mailbox')}</Label>
        <Input bind:value={formData.mailbox} placeholder="INBOX" />
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('channel.mailboxHelp')}</p>
      </div>

      <div class="space-y-3">
        <div class="p-3 rounded" style="background-color: var(--ds-surface-raised);">
          <Checkbox
            bind:checked={formData.mark_as_read}
            label={t('channel.markAsRead')}
            hint={t('channel.markAsReadHelp')}
            size="small"
          />
        </div>

        <div class="p-3 rounded" style="background-color: var(--ds-surface-raised);">
          <Checkbox
            bind:checked={formData.delete_after_process}
            label={t('channel.deleteAfterProcess')}
            hint={t('channel.deleteAfterProcessHelp')}
            size="small"
          />
        </div>
      </div>
    </div>

    <!-- Enable Email Channel Toggle - Prominent -->
    <div
      class="flex items-center justify-between p-4 rounded-lg border-2 transition-colors cursor-pointer"
      style="background-color: {formData.enabled ? 'var(--ds-background-success)' : 'var(--ds-surface-raised)'}; border-color: {formData.enabled ? 'var(--ds-border-success)' : 'var(--ds-border)'};"
      onclick={() => formData.enabled = !formData.enabled}
      role="button"
      tabindex="0"
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); formData.enabled = !formData.enabled; }}}
    >
      <div class="flex items-center gap-3">
        <div
          class="w-10 h-6 rounded-full relative transition-colors"
          style="background-color: {formData.enabled ? 'var(--ds-background-success-bold)' : 'var(--ds-background-neutral)'};"
        >
          <div
            class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform"
            style="transform: translateX({formData.enabled ? '22px' : '4px'});"
          ></div>
        </div>
        <div>
          <div class="text-sm font-semibold" style="color: var(--ds-text);">
            {t('channel.enableEmail', 'Enable Email Channel')}
          </div>
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            {formData.enabled ? t('channel.emailIsActive', 'Email channel is active and processing emails') : t('channel.emailIsInactive', 'Email channel is currently disabled')}
          </div>
        </div>
      </div>
      <div
        class="px-3 py-1 rounded-full text-xs font-semibold"
        style="background-color: {formData.enabled ? 'var(--ds-background-success-bold)' : 'var(--ds-background-neutral)'}; color: {formData.enabled ? 'white' : 'var(--ds-text-subtle)'};"
      >
        {formData.enabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
      </div>
    </div>
  </div>
</div>
