<script>
  import IconX from '@tabler/icons-svelte-runes/icons/x';
  import { t } from '../../stores/i18n.svelte.js';
  import { channels } from '../../api/channels.js';
  import AlertBox from '../../components/AlertBox.svelte';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Radio from '../../components/Radio.svelte';
	import TextField from '../../components/TextField.svelte';

  let {
    channelId,
    formData = $bindable({
      url: '',
      secret: '',
      headers: [],
      scope_type: 'all',
      workspace_ids: [],
      collection_ids: [],
      auto_trigger: false,
      subscribed_events: [],
      enabled: false
    }),
    isPluginOwned = false,
    pluginName = ''
  } = $props();

  let webhookTestResult = $state(null);
  let loading = $state(false);

  // Clear stale test result when the user edits any tested field.
  $effect(() => {
    formData.url;
    formData.secret;
    formData.headers;
    formData.scope_type;
    formData.workspace_ids;
    formData.collection_ids;
    formData.auto_trigger;
    formData.subscribed_events;
    if (webhookTestResult && !webhookTestResult.loading) {
      webhookTestResult = null;
    }
  });

  // Available webhook events
  const webhookEvents = [
    { id: 'item.created', labelKey: 'channel.itemCreated', categoryKey: 'channel.items' },
    { id: 'item.updated', labelKey: 'channel.itemUpdated', categoryKey: 'channel.items' },
    { id: 'item.deleted', labelKey: 'channel.itemDeleted', categoryKey: 'channel.items' },
    { id: 'item.assigned', labelKey: 'channel.itemAssigned', categoryKey: 'channel.items' },
    { id: 'status.changed', labelKey: 'channel.statusChanged', categoryKey: 'channel.items' }
  ];

  function addWebhookHeader() {
    formData.headers = [...formData.headers, { key: '', value: '' }];
  }

  function removeWebhookHeader(index) {
    formData.headers = formData.headers.filter((_, i) => i !== index);
  }

  function toggleWebhookEvent(eventId) {
    if (formData.subscribed_events.includes(eventId)) {
      formData.subscribed_events = formData.subscribed_events.filter(e => e !== eventId);
    } else {
      formData.subscribed_events = [...formData.subscribed_events, eventId];
    }
  }

  async function testWebhookSettings() {
    if (loading) return;
    if (!channelId || !formData.url) {
      webhookTestResult = { success: false, message: t('channel.pleaseEnterUrl') };
      return;
    }

    try {
      new URL(formData.url);
    } catch {
      webhookTestResult = { success: false, message: t('channel.pleaseEnterValidUrl') };
      return;
    }

    webhookTestResult = { success: true, message: t('channel.sendingTestWebhook'), loading: true };
    loading = true;

    try {
      const configData = getConfig();
      const result = await channels.testConfig(channelId, configData);
      if (result.success) {
        webhookTestResult = {
          success: true,
          message: t('channel.testWebhookSent'),
          loading: false
        };
      } else {
        webhookTestResult = {
          success: false,
          message: `${t('channel.testWebhookFailed')}: ${result.message || 'Unknown error'}`,
          loading: false
        };
      }
    } catch (error) {
      console.error('Failed to test webhook:', error);
      webhookTestResult = {
        success: false,
        message: t('channel.testWebhookFailed') + ': ' + (error.message || error),
        loading: false
      };
    } finally {
      loading = false;
    }
  }

  export function validate() {
    if (!formData.url?.trim()) {
      return { valid: false, message: t('channel.webhookUrlRequired') };
    }
    return { valid: true };
  }

  export function getConfig() {
    formData.headers = formData.headers.filter(h => h.key && h.key.trim());
    const headersObj = {};
    formData.headers.forEach(h => {
      headersObj[h.key.trim()] = h.value || '';
    });

    return {
      webhook_url: formData.url,
      webhook_secret: formData.secret || undefined,
      webhook_headers: headersObj,
      webhook_scope_type: formData.scope_type,
      webhook_workspace_ids: formData.scope_type === 'workspaces' ? formData.workspace_ids : undefined,
      webhook_auto_trigger: formData.auto_trigger,
      webhook_subscribed_events: formData.auto_trigger ? formData.subscribed_events : undefined
    };
  }

  export function clearSecret() {
    formData.secret = '';
  }
</script>

<div class="pt-6 border-t" style="border-color: var(--ds-border);">
  <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.webhookConfiguration')}</h4>

  {#if isPluginOwned}
    <div class="p-4 rounded-lg border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        {t('channel.managedByPlugin', { pluginName })}
      </p>
    </div>
  {:else}
    <div class="space-y-6">
      <div class="space-y-4">
        <div>
          <TextField
            label={t('channel.webhookUrl')}
            required
            labelColor="default"
            type="url"
            placeholder="https://your-server.com/webhook"
            bind:value={formData.url}
          />
        </div>

        <div>
          <TextField
            label={t('channel.secretOptional')}
            labelColor="default"
            type="password"
            placeholder={t('channel.secretPlaceholder')}
            bind:value={formData.secret}
          />
          <DescriptionText>
            {t('channel.secretHelp')}
          </DescriptionText>
        </div>

        <!-- Custom Headers -->
        <div>
          <div class="flex items-center justify-between mb-2">
            <Label color="default">{t('channel.customHeaders')}</Label>
            <Button type="button" variant="ghost" size="small" onclick={addWebhookHeader}>
              {t('channel.addHeader')}
            </Button>
          </div>
          {#if formData.headers.length > 0}
            <div class="space-y-2">
              {#each formData.headers as header, index}
                <div class="flex gap-2 items-center">
                  <Input bind:value={header.key} placeholder={t('channel.headerName')} class="flex-1" />
                  <Input bind:value={header.value} placeholder={t('channel.headerValue')} class="flex-1" />
                  <Button type="button" variant="ghost" size="small" onclick={() => removeWebhookHeader(index)}>
                    <IconX class="w-4 h-4" />
                  </Button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <!-- Scope Configuration -->
      <div class="pt-4 border-t" style="border-color: var(--ds-border);">
        <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('channel.scope')}</h5>
        <div class="space-y-3">
          <label class="flex items-center gap-2 cursor-pointer">
            <Radio name="webhookScope" value="all" bind:groupValue={formData.scope_type} />
            <span class="text-sm" style="color: var(--ds-text);">{t('channel.allItems')}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <Radio name="webhookScope" value="workspaces" bind:groupValue={formData.scope_type} />
            <span class="text-sm" style="color: var(--ds-text);">{t('channel.specificWorkspaces')}</span>
          </label>
          {#if formData.scope_type === 'workspaces'}
            <div class="ml-6">
              <WorkspacePicker bind:value={formData.workspace_ids} placeholder={t('channel.selectWorkspaces')} />
            </div>
          {/if}
        </div>
      </div>

      <!-- Event Triggers -->
      <div class="pt-4 border-t" style="border-color: var(--ds-border);">
        <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('channel.automaticTriggers')}</h5>
        <div class="p-3 rounded" style="background-color: var(--ds-surface-raised);">
          <Checkbox
            bind:checked={formData.auto_trigger}
            label={t('channel.enableAutoTriggers')}
            hint={t('channel.autoTriggersHelp')}
            size="small"
          />
        </div>

        {#if formData.auto_trigger}
          <div class="mt-4 space-y-4">
            {#each ['channel.items'] as categoryKey}
              <div>
                <h6 class="text-xs font-medium uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">{t(categoryKey)}</h6>
                <div class="grid grid-cols-2 gap-2">
                  {#each webhookEvents.filter(e => e.categoryKey === categoryKey) as event}
                    <div class="p-2 rounded" style="background-color: var(--ds-surface);">
                      <Checkbox
                        checked={formData.subscribed_events.includes(event.id)}
                        onchange={() => toggleWebhookEvent(event.id)}
                        label={t(event.labelKey)}
                        size="small"
                      />
                    </div>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <div class="flex items-center justify-between pt-4 border-t" style="border-color: var(--ds-border);">
        <div>
          <div class="text-sm font-medium" style="color: var(--ds-text);">
            {t('channel.enableWebhook', 'Enable Webhook Channel')}
          </div>
          <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {formData.enabled
              ? t('channel.webhookIsActive', 'Webhook channel is active')
              : t('channel.webhookIsInactive', 'Webhook channel is currently disabled')}
          </div>
        </div>
        <Toggle bind:checked={formData.enabled} />
      </div>
    </div>

    <!-- Test Webhook Section -->
    <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.testWebhook')}</h5>
      <div class="flex gap-2 mb-4">
        <Button onclick={testWebhookSettings} variant="secondary" disabled={!formData.url || loading}>
          {t('channel.sendTestWebhook')}
        </Button>
      </div>

      {#if webhookTestResult}
        {#if webhookTestResult.loading}
          <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
            <Spinner size="sm" />
            <span>{webhookTestResult.message}</span>
          </div>
        {:else}
          <AlertBox
            variant={webhookTestResult.success ? 'success' : 'error'}
            message={webhookTestResult.message}
          />
        {/if}
      {/if}
    </div>
  {/if}
</div>
