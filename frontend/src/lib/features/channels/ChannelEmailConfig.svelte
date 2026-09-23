<script>
  import { IconCheck } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import WorkspaceSelector from '../../workspaces/WorkspaceSelector.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import { publicBaseURL } from '../../runtime/contextPath.js';
  import { isSystemAdmin } from '../../stores/permissions.svelte.js';
	import TextField from '../../components/TextField.svelte';
	import SelectField from '../../components/SelectField.svelte';

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
      connected_oauth_provider_type: '',
      connected_oauth_client_id: '',
      connected_oauth_tenant_id: '',
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
      rate_limit_per_hour: null,
      enabled: false
    }),
    workspaces = [],
    itemTypes = [],
    loading = $bindable(false),
    onLoadItemTypes = () => {},
    onSaveBeforeOAuth = async () => {},
    onOAuthStartFailed = async () => {},
    onToast = () => {}
  } = $props();

  let oauthIdentityChanged = $derived(
    formData.oauth_connected && (
      formData.oauth_provider_type !== formData.connected_oauth_provider_type ||
      formData.oauth_client_id.trim() !== formData.connected_oauth_client_id.trim() ||
      (formData.oauth_provider_type === 'microsoft' &&
        formData.oauth_tenant_id.trim() !== formData.connected_oauth_tenant_id.trim())
    )
  );
  let oauthIsConnected = $derived(formData.oauth_connected && !oauthIdentityChanged);

  async function startOAuthFlow() {
    if (!$isSystemAdmin || !channelId) return;

    if (!formData.oauth_client_id) {
      onToast('Please enter OAuth client ID');
      return;
    }

    let restoreEnabled = false;
    try {
      loading = true;
      restoreEnabled = await onSaveBeforeOAuth();
      const result = await api.channels.startEmailOAuth(channelId, restoreEnabled);
      if (result.auth_url) {
        window.location.href = result.auth_url;
      } else {
        throw new Error('OAuth start did not return an authorization URL');
      }
    } catch (error) {
      try {
        await onOAuthStartFailed(restoreEnabled);
      } catch (restoreError) {
        console.error('Failed to restore email channel after OAuth start failure:', restoreError);
      }
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
      if (!oauthIsConnected && !formData.oauth_client_secret?.trim()) {
        return { valid: false, message: t('channel.clientSecretRequired') };
      }
    }

    if (!formData.workspace_id) {
      return { valid: false, message: t('channel.targetWorkspaceRequired') };
    }
    if (!formData.item_type_id) {
      return { valid: false, message: t('channel.itemTypeRequired') };
    }
    if (formData.rate_limit_per_hour !== null && formData.rate_limit_per_hour !== '' && Number(formData.rate_limit_per_hour) < 0) {
      return { valid: false, message: t('channel.rateLimitInvalid') };
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
      // null = default cap, 0 = unlimited, n = n per sender per hour
      email_rate_limit_per_hour:
        formData.rate_limit_per_hour === null || formData.rate_limit_per_hour === ''
          ? null
          : Number(formData.rate_limit_per_hour)
    };

    if (formData.auth_method === 'oauth') {
      return {
        ...baseConfig,
        email_oauth_provider_type: formData.oauth_provider_type,
        email_oauth_client_id: formData.oauth_client_id.trim(),
        email_oauth_client_secret: formData.oauth_client_secret || undefined,
        email_oauth_tenant_id: formData.oauth_provider_type === 'microsoft' ? formData.oauth_tenant_id.trim() : undefined
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
          <DescriptionText as="div">
            {t('channel.usernameAndPassword')}
          </DescriptionText>
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
          <DescriptionText as="div">
            {t('channel.microsoftOrGoogle')}
          </DescriptionText>
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
            <TextField
              label={t('channel.clientId')}
              required
              labelColor="default"
              placeholder="Application (client) ID"
              bind:value={formData.oauth_client_id}
            />
          </div>
          <div>
            <TextField
              label={t('channel.clientSecret')}
              required
              labelColor="default"
              type="password"
              placeholder={oauthIsConnected ? t('channel.leaveBlankToKeep') : 'Client secret value'}
              bind:value={formData.oauth_client_secret}
            />
          </div>
        </div>

        {#if formData.oauth_provider_type === 'microsoft'}
          <div>
            <TextField
              label={t('channel.tenantId')}
              labelColor="default"
              placeholder="common (multi-tenant) or specific tenant ID"
              bind:value={formData.oauth_tenant_id}
            />
            <DescriptionText>
              {t('channel.tenantIdHelp')}
            </DescriptionText>
          </div>
        {/if}

        <!-- Connection Status -->
        {#if oauthIsConnected}
          <div class="p-4 rounded-lg border" style="background: var(--ds-background-success-subtle); border-color: var(--ds-border-success);">
            <div class="flex items-center gap-3">
              <IconCheck class="w-5 h-5" style="color: var(--ds-icon-success);" />
              <div class="flex-1">
                <div class="font-medium" style="color: var(--ds-text);">{t('channel.connected')}</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  {formData.oauth_email}
                </div>
              </div>
              {#if $isSystemAdmin}
                <Button variant="ghost" size="small" onclick={startOAuthFlow} disabled={loading}>
                  {t('channel.reconnect')}
                </Button>
              {/if}
            </div>
          </div>
        {:else if formData.oauth_client_id && $isSystemAdmin}
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
            {publicBaseURL()}/api/channels/inline-oauth/callback
          </code>
        </div>
      </div>
    {:else}
      <!-- Basic IMAP Configuration -->
      <div class="space-y-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.imapConnection')}</h5>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <TextField
              label={t('channel.imapHost')}
              required
              labelColor="default"
              placeholder="imap.example.com"
              bind:value={formData.imap_host}
            />
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <TextField
                label={t('channel.port')}
                labelColor="default"
                type="number"
                placeholder="993"
                bind:value={formData.imap_port}
              />
            </div>
            <div>
              <SelectField
                label={t('channel.encryption')}
                labelColor="default"
                options={[{ value: 'ssl', label: 'SSL/TLS (implicit)' }, { value: 'starttls', label: 'STARTTLS' }]}
                bind:value={formData.imap_encryption}
              />
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <TextField
              label={t('channel.username')}
              required
              labelColor="default"
              placeholder="user@example.com"
              bind:value={formData.imap_username}
            />
          </div>
          <div>
            <TextField
              label={t('channel.password')}
              required
              labelColor="default"
              type="password"
              placeholder="Enter password to update"
              bind:value={formData.imap_password}
            />
            <DescriptionText>{t('channel.leaveBlankPassword')}</DescriptionText>
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
          <SelectField
            label={t('channel.itemType')}
            required
            labelColor="default"
            disabled={!formData.workspace_id}
            options={[{ value: null, label: t('channel.selectItemType') }, ...itemTypes.map(type => ({ value: type.id, label: type.name }))]}
            bind:value={formData.item_type_id}
          />
          {#if !formData.workspace_id}
            <DescriptionText>{t('channel.selectWorkspaceFirst')}</DescriptionText>
          {/if}
        </div>
      </div>
    </div>

    <!-- Processing Options -->
    <div class="pt-4 border-t space-y-4" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-medium" style="color: var(--ds-text);">{t('channel.processingOptions')}</h5>

      <div>
        <TextField
          label={t('channel.mailbox')}
          labelColor="default"
          placeholder="INBOX"
          bind:value={formData.mailbox}
        />
        <DescriptionText>{t('channel.mailboxHelp')}</DescriptionText>
      </div>

      <div>
        <TextField
          label={t('channel.rateLimitPerHour')}
          labelColor="default"
          type="number"
          min="0"
          placeholder="100"
          bind:value={formData.rate_limit_per_hour}
        />
        <DescriptionText>{t('channel.rateLimitHelp')}</DescriptionText>
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

    <div class="flex items-center justify-between">
      <div>
        <div class="text-sm font-medium" style="color: var(--ds-text);">
          {t('channel.enableEmail', 'Enable Email Channel')}
        </div>
        <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {formData.enabled
            ? t('channel.emailIsActive', 'Email channel is active and processing emails')
            : t('channel.emailIsInactive', 'Email channel is currently disabled')}
        </div>
      </div>
      <Toggle bind:checked={formData.enabled} />
    </div>
  </div>
</div>
