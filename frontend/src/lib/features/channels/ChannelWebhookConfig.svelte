<script>
  import { X, Check } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import CollectionPicker from '../../pickers/CollectionPicker.svelte';

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
      subscribed_events: []
    }),
    isPluginOwned = false,
    pluginName = '',
    onSave = () => {}
  } = $props();

  let webhookTestResult = $state(null);
  let loading = $state(false);

  // Available webhook events
  const webhookEvents = [
    { id: 'item.created', labelKey: 'channel.itemCreated', categoryKey: 'channel.items' },
    { id: 'item.updated', labelKey: 'channel.itemUpdated', categoryKey: 'channel.items' },
    { id: 'item.deleted', labelKey: 'channel.itemDeleted', categoryKey: 'channel.items' },
    { id: 'item.assigned', labelKey: 'channel.itemAssigned', categoryKey: 'channel.items' },
    { id: 'status.changed', labelKey: 'channel.statusChanged', categoryKey: 'channel.items' },
    { id: 'comment.created', labelKey: 'channel.commentCreated', categoryKey: 'channel.comments' },
    { id: 'comment.updated', labelKey: 'channel.commentUpdated', categoryKey: 'channel.comments' },
    { id: 'comment.deleted', labelKey: 'channel.commentDeleted', categoryKey: 'channel.comments' },
    { id: 'item.linked', labelKey: 'channel.itemLinked', categoryKey: 'channel.links' },
    { id: 'item.unlinked', labelKey: 'channel.itemUnlinked', categoryKey: 'channel.links' }
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

    try {
      const configData = getConfig();
      await api.channels.updateConfig(channelId, configData);

      const result = await api.channels.test(channelId);
      if (result.success) {
        webhookTestResult = {
          success: true,
          message: t('channel.testWebhookSent'),
          loading: false
        };
        onSave();
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
    }
  }

  export function validate() {
    if (!formData.url?.trim()) {
      return { valid: false, message: t('channel.webhookUrlRequired') };
    }
    return { valid: true };
  }

  export function getConfig() {
    const headersObj = {};
    formData.headers.forEach(h => {
      if (h.key && h.key.trim()) {
        headersObj[h.key.trim()] = h.value || '';
      }
    });

    return {
      webhook_url: formData.url,
      webhook_secret: formData.secret || undefined,
      webhook_headers: Object.keys(headersObj).length > 0 ? headersObj : undefined,
      webhook_scope_type: formData.scope_type,
      webhook_workspace_ids: formData.scope_type === 'workspaces' ? formData.workspace_ids : undefined,
      webhook_collection_ids: formData.scope_type === 'collections' ? formData.collection_ids : undefined,
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
          <Label color="default" required class="mb-2">{t('channel.webhookUrl')}</Label>
          <Input type="url" bind:value={formData.url} required placeholder="https://your-server.com/webhook" />
        </div>

        <div>
          <Label color="default" class="mb-2">{t('channel.secretOptional')}</Label>
          <Input type="password" bind:value={formData.secret} placeholder={t('channel.secretPlaceholder')} />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('channel.secretHelp')}
          </p>
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
        <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('channel.scope')}</h5>
        <div class="space-y-3">
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="radio" name="webhookScope" value="all" bind:group={formData.scope_type} class="w-4 h-4" />
            <span class="text-sm" style="color: var(--ds-text);">{t('channel.allItems')}</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="radio" name="webhookScope" value="workspaces" bind:group={formData.scope_type} class="w-4 h-4" />
            <span class="text-sm" style="color: var(--ds-text);">{t('channel.specificWorkspaces')}</span>
          </label>
          {#if formData.scope_type === 'workspaces'}
            <div class="ml-6">
              <WorkspacePicker bind:value={formData.workspace_ids} placeholder={t('channel.selectWorkspaces')} />
            </div>
          {/if}
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="radio" name="webhookScope" value="collections" bind:group={formData.scope_type} class="w-4 h-4" />
            <span class="text-sm" style="color: var(--ds-text);">{t('channel.specificCollections')}</span>
          </label>
          {#if formData.scope_type === 'collections'}
            <div class="ml-6">
              <CollectionPicker bind:value={formData.collection_ids} placeholder={t('channel.selectCollections')} />
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
            {#each ['channel.items', 'channel.comments', 'channel.links'] as categoryKey}
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
