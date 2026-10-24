<script>
  import IconCheck from '@tabler/icons-svelte-runes/icons/check';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { channels } from '../../api/channels.js';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import ChannelIntakesSection from './ChannelIntakesSection.svelte';
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
      oauth_mailbox: '',
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
      processing_disposition: 'mark_read',
      rate_limit_per_hour: null,
      enabled: false
    }),
    workspaces = [],
    portals = [],
    loading = $bindable(false),
    onSaveBeforeOAuth = async () => {},
    onOAuthStartFailed = async () => {}
  } = $props();

  // Older callers (and stored configs) carry the two legacy booleans instead of
  // the single disposition. Derive it once so the select always has a value.
  if (formData.processing_disposition === undefined) {
    formData.processing_disposition = formData.delete_after_process
      ? 'delete'
      : formData.mark_as_read === false
        ? 'leave'
        : 'mark_read';
  }

  let oauthIdentityChanged = $derived(
    formData.oauth_connected && (
      formData.oauth_provider_type !== formData.connected_oauth_provider_type ||
      formData.oauth_client_id.trim() !== formData.connected_oauth_client_id.trim() ||
      formData.oauth_mailbox.trim() !== formData.connected_oauth_mailbox.trim() ||
      (formData.oauth_provider_type === 'microsoft' &&
        formData.oauth_tenant_id.trim() !== formData.connected_oauth_tenant_id.trim())
    )
  );
  let oauthIsConnected = $derived(formData.oauth_connected && !oauthIdentityChanged);

  async function startOAuthFlow() {
    if (!$isSystemAdmin || !channelId) return;

    if (!formData.oauth_client_id) {
      errorToast('Please enter OAuth client ID');
      return;
    }

    if (!formData.oauth_mailbox?.trim()) {
      errorToast(t('channel.oauthMailboxRequired'));
      return;
    }

    let restoreEnabled = false;
    try {
      loading = true;
      restoreEnabled = await onSaveBeforeOAuth();
      const result = await channels.startEmailOAuth(channelId, restoreEnabled);
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
      errorToast('Failed to start OAuth: ' + (error.message || error));
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
      if (!formData.oauth_mailbox?.trim()) {
        return { valid: false, message: t('channel.oauthMailboxRequired') };
      }
      if (!oauthIsConnected && !formData.oauth_client_secret?.trim()) {
        return { valid: false, message: t('channel.clientSecretRequired') };
      }
    }

    return { valid: true };
  }

  export function getConfig() {
    // The channel config carries the connection plus the mailbox-level
    // post-processing default. Routing (folder, target, request/item type, rate
    // limit) and per-intake disposition overrides live on intakes.
    const baseConfig = {
      email_auth_method: formData.auth_method,
      email_processing_disposition: formData.processing_disposition || 'mark_read'
    };

    if (formData.auth_method === 'oauth') {
      return {
        ...baseConfig,
        email_oauth_provider_type: formData.oauth_provider_type,
        email_oauth_client_id: formData.oauth_client_id.trim(),
        email_oauth_client_secret: formData.oauth_client_secret || undefined,
        email_oauth_mailbox: formData.oauth_mailbox.trim(),
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

        <div>
          <TextField
            label={t('channel.oauthMailbox')}
            required
            labelColor="default"
            placeholder="support@example.com"
            dataTestid="channel-oauth-mailbox"
            bind:value={formData.oauth_mailbox}
          />
          <DescriptionText>
            {t('channel.oauthMailboxHelp')}
          </DescriptionText>
        </div>

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
              <Button variant="primary" onclick={startOAuthFlow} disabled={loading} dataTestid="channel-oauth-connect">
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

    <!-- Mailbox-level post-processing default; an intake may override it. -->
    <div class="pt-4 border-t" style="border-color: var(--ds-border);">
      <SelectField
        label={t('channel.processingDisposition')}
        labelColor="default"
        id="email-processing-disposition"
        options={[
          { value: 'leave', label: t('channel.dispositionLeave') },
          { value: 'mark_read', label: t('channel.dispositionMarkRead') },
          { value: 'delete', label: t('channel.dispositionDelete') }
        ]}
        bind:value={formData.processing_disposition}
      />
      <DescriptionText>{t('channel.dispositionHelp')}</DescriptionText>
    </div>

    <!-- Intakes: routing is separate from the connection. One mailbox can feed
         several intakes via distinct folders. -->
    <ChannelIntakesSection {channelId} {workspaces} {portals} />

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
      <Toggle bind:checked={formData.enabled} dataTestid="channel-email-enable" />
    </div>
  </div>
</div>
