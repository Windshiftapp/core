<script>
  import { onMount } from 'svelte';
  import { IconArrowLeft, IconExternalLink, IconSettings, IconUsers } from '@tabler/icons-svelte-runes';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { channelCategoriesStore } from '../../stores/channelCategories.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import ChannelPortalConfig from './ChannelPortalConfig.svelte';
  import ChannelManagersTab from '../../settings/ChannelManagersTab.svelte';

  let channel = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let activeTab = $state('settings');

  let portalConfigRef = $state(null);

  let channelFormData = $state({
    name: '',
    description: '',
    category_id: null,
  });

  let portalFormData = $state({
    slug: '',
    workspace_ids: [],
    enabled: false,
    title: '',
    description: '',
    registration_mode: 'open',
    allowed_domains: '',
  });

  let channelId = $derived(parseInt($currentRoute.path.match(/\/admin\/channels\/(\d+)\/portal/)?.[1]));

  onMount(async () => {
    await channelCategoriesStore.init();
    await loadChannel();
  });

  function parseChannelConfig(config) {
    if (!config) return {};
    if (typeof config === 'string') {
      if (config.trim() === '') return {};
      try { return JSON.parse(config); } catch { return {}; }
    }
    return config || {};
  }

  async function loadChannel() {
    try {
      loading = true;
      channel = await api.channels.get(channelId);

      channelFormData = {
        name: channel.name || '',
        description: channel.description || '',
        category_id: channel.category_id || null,
      };

      const config = parseChannelConfig(channel.config);
      portalFormData = {
        slug: config.portal_slug || '',
        workspace_ids: config.portal_workspace_ids || [],
        enabled: channel.status === 'enabled',
        title: config.portal_title || '',
        description: config.portal_description || '',
        registration_mode: config.portal_registration_mode === 'manual' ? 'manual' : 'open',
        allowed_domains: Array.isArray(config.portal_allowed_domains)
          ? config.portal_allowed_domains.join(', ')
          : '',
      };
    } catch (err) {
      console.error('Failed to load channel:', err);
      errorToast(t('channel.failedToLoad', 'Failed to load channel'));
    } finally {
      loading = false;
    }
  }

  async function handleSaveSettings() {
    if (!channel) return;

    if (portalConfigRef) {
      const validation = portalConfigRef.validate();
      if (!validation.valid) {
        errorToast(validation.message);
        return;
      }
    }

    try {
      saving = true;

      await api.channels.update(channel.id, {
        id: channel.id,
        type: channel.type,
        direction: channel.direction,
        is_default: channel.is_default,
        name: channelFormData.name,
        description: channelFormData.description,
        category_id: channelFormData.category_id,
      });

      if (portalConfigRef) {
        const existingConfig = parseChannelConfig(channel.config);
        const configData = {
          ...existingConfig,
          ...portalConfigRef.getConfig(),
        };
        await api.channels.updateConfig(channel.id, configData);
      }

      channel = await api.channels.get(channelId);
      successToast(t('common.saved'));
    } catch (err) {
      console.error('Failed to save:', err);
      errorToast(err.message || t('common.error'));
    } finally {
      saving = false;
    }
  }

  const tabs = [
    { id: 'settings', label: () => t('channel.configuration'), icon: IconSettings },
    { id: 'managers', label: () => t('channel.managers'), icon: IconUsers },
  ];
</script>

{#if loading}
  <div class="flex-1 flex items-center justify-center py-24">
    <Spinner />
  </div>
{:else if channel}
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="px-16 pt-8 pb-0">
      <div class="mb-4">
        <a
          href="/admin/channels"
          class="inline-flex items-center gap-1 text-sm transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
          onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
        >
          <IconArrowLeft class="w-4 h-4" />
          {t('channels.title')}
        </a>
      </div>

      <div class="flex items-center justify-between mb-6">
        <div>
          <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
            {channel.name}
          </h1>
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
            {t('channels.portal', 'Portal Channel')}
          </p>
        </div>
        {#if portalFormData.slug}
          <Button
            onclick={() => window.open(`/portal/${portalFormData.slug}`, '_blank')}
            variant="default"
            size="small"
            icon={IconExternalLink}
          >
            {t('channel.openPortal')}
          </Button>
        {/if}
      </div>

      <!-- Tab Navigation -->
      <nav class="flex gap-6 border-b" style="border-color: var(--ds-border);">
        {#each tabs as tab}
          <button
            onclick={() => activeTab = tab.id}
            class="relative py-3 text-sm font-medium transition-colors {
              activeTab === tab.id
                ? 'text-[var(--ds-interactive)]'
                : 'text-[var(--ds-text-subtle)] hover:text-[var(--ds-text)]'
            }"
          >
            <div class="flex items-center gap-2">
              <tab.icon class="w-4 h-4" />
              <span>{tab.label()}</span>
            </div>
            {#if activeTab === tab.id}
              <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]"></div>
            {/if}
          </button>
        {/each}
      </nav>
    </div>

    <!-- Tab Content -->
    <div class="flex-1 overflow-y-auto">
      {#if activeTab === 'settings'}
        <div class="px-16 py-8 max-w-3xl">
          <!-- Basic Info -->
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
                  <Select
                    bind:value={channelFormData.category_id}
                    options={[
                      { value: null, label: t('channel.noCategory') },
                      ...$channelCategoriesStore.map(c => ({ value: c.id, label: c.name })),
                    ]}
                  />
                </div>
              </div>
              <div>
                <Label color="default" class="mb-2">{t('channel.description')}</Label>
                <Textarea bind:value={channelFormData.description} rows={2} placeholder={t('channel.briefDescription')} />
              </div>
            </div>
          </div>

          <!-- Portal-specific Config -->
          <ChannelPortalConfig
            bind:this={portalConfigRef}
            bind:formData={portalFormData}
          />

          <!-- Save Button -->
          <div class="mt-8 flex justify-end">
            <Button
              onclick={handleSaveSettings}
              variant="primary"
              disabled={saving}
            >
              {saving ? t('common.saving') : t('channel.saveChanges')}
            </Button>
          </div>
        </div>
      {:else if activeTab === 'managers'}
        <div class="px-16 py-8">
          <ChannelManagersTab
            channelId={channel.id}
            channelName={channel.name}
            isDefault={channel.is_default}
          />
        </div>
      {/if}
    </div>
  </div>
{/if}
