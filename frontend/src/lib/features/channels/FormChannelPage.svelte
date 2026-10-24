<script>
  import { onMount, onDestroy } from 'svelte';
  import IconSettings from '@tabler/icons-svelte-runes/icons/settings';
  import IconForms from '@tabler/icons-svelte-runes/icons/forms';
  import IconCode from '@tabler/icons-svelte-runes/icons/code';
  import IconUsers from '@tabler/icons-svelte-runes/icons/users';
  import { currentRoute, navigate } from '../../router.js';
  import { channels } from '../../api/channels.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { isSystemAdmin } from '../../stores/permissions.svelte.js';
  import { channelCategoriesStore } from '../../stores/channelCategories.js';
  import { formBuilderStore } from '../../stores/formBuilderStore.svelte.js';
  import ChannelAdminShell from './ChannelAdminShell.svelte';
  import ChannelAdminSettings from './ChannelAdminSettings.svelte';
  import FormBuilder from './FormBuilder.svelte';
  import ChannelFormConfig from './ChannelFormConfig.svelte';
  import FormIntegrationPanel from './FormIntegrationPanel.svelte';
  import ChannelManagersTab from '../../settings/ChannelManagersTab.svelte';
  import CreateFormModal from './CreateFormModal.svelte';
  import {
    channelBasicFormData,
    parseChannelConfig,
    prepareFormChannelForWorkspace,
    saveChannelSettings,
  } from './channelAdmin.js';

  let channel = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let activeTab = $state('forms');
  let showCreateModal = $state(false);
  let loadSequence = 0;

  let formConfigRef = $state(null);

  let channelFormData = $state({
    name: '',
    description: '',
    category_id: null,
  });

  let formChannelFormData = $state({
    slug: '',
    workspace_ids: [],
    enabled: false,
    theme: 'light',
    brand_color: '#14b8a6',
    logo_url: '',
    success_message: '',
    redirect_url: '',
  });

  let channelId = $derived(parseInt($currentRoute.path.match(/\/admin\/channels\/(\d+)\/forms/)?.[1]));

  onMount(async () => {
    await channelCategoriesStore.init();
  });

  $effect(() => {
    const id = channelId;
    if (Number.isInteger(id) && id > 0) {
      formBuilderStore.reset();
      void loadChannel(id);
    }
  });

  onDestroy(() => {
    formBuilderStore.reset();
  });

  async function loadChannel(id) {
    const requestSequence = ++loadSequence;
    try {
      loading = true;
      const loadedChannel = await channels.get(id);
      if (loadedChannel.type !== 'form' || loadedChannel.direction !== 'inbound') {
        throw new Error('This route requires an inbound form channel');
      }
      const config = parseChannelConfig(loadedChannel.config);
      if (requestSequence !== loadSequence) return;
      channel = loadedChannel;

      channelFormData = channelBasicFormData(channel);
      formChannelFormData = {
        slug: config.form_slug || '',
        workspace_ids: config.form_workspace_ids || [],
        enabled: channel.status === 'enabled',
        theme: config.form_theme || 'light',
        brand_color: config.form_brand_color || '#14b8a6',
        logo_url: config.form_logo_url || '',
        success_message: config.form_success_message || '',
        redirect_url: config.form_redirect_url || '',
      };
    } catch (err) {
      if (requestSequence !== loadSequence) return;
      console.error('Failed to load channel:', err);
      channel = null;
      errorToast(err.message || 'Failed to load channel');
    } finally {
      if (requestSequence === loadSequence) loading = false;
    }
  }

  async function handleSaveSettings() {
    if (!channel) return;

    if (formConfigRef) {
      const validation = formConfigRef.validate();
      if (!validation.valid) {
        errorToast(validation.message);
        return;
      }
    }

    try {
      saving = true;

      await saveChannelSettings({
        channel,
        channelFormData,
        configRef: formConfigRef,
        enabled: formChannelFormData.enabled,
      });

      await loadChannel(channelId);
      successToast(t('common.saved'));
    } catch (err) {
      console.error('Failed to save:', err);
      errorToast(err.message || t('common.error'));
    } finally {
      saving = false;
    }
  }

  function handleFormCreated() {
    formBuilderStore.loadForms(channelId);
  }

  async function prepareFormCreation(workspaceId) {
    const prepared = await prepareFormChannelForWorkspace({
      channel,
      workspaceIds: formChannelFormData.workspace_ids,
      workspaceId,
    });
    formChannelFormData.workspace_ids = prepared.workspaceIds;
    formChannelFormData.enabled = prepared.status === 'enabled';
    channel = { ...channel, status: prepared.status };
  }

  let tabs = $derived([
    { id: 'forms', label: () => t('forms.title'), icon: IconForms },
    { id: 'settings', label: () => t('channel.configuration'), icon: IconSettings },
    ...(formBuilderStore.forms.length > 0
      ? [{ id: 'integration', label: () => t('forms.integration.title'), icon: IconCode }]
      : []),
    ...($isSystemAdmin
      ? [{ id: 'managers', label: () => t('channel.managers'), icon: IconUsers }]
      : []),
  ]);
</script>

<ChannelAdminShell
  {loading}
  {channel}
  bind:activeTab
  {tabs}
  subtitle={t('channels.form', 'Form Channel')}
  openUrl={formChannelFormData.slug ? `/forms/${formChannelFormData.slug}` : ''}
  openLabel={t('channel.openForm')}
>
  {#snippet children(tabId)}
    {#if tabId === 'forms'}
      <div class="px-16 py-8">
        <FormBuilder
          {channelId}
          channelSlug={formChannelFormData.slug}
          channelBrandColor={formChannelFormData.brand_color}
          channelWorkspaceIds={formChannelFormData.workspace_ids}
          onBack={() => navigate('/admin/channels')}
          onCreateForm={() => showCreateModal = true}
          onOpenSettings={() => activeTab = 'settings'}
          embedded={false}
        />
      </div>
    {:else if tabId === 'settings'}
      <ChannelAdminSettings bind:channelFormData {saving} onSave={handleSaveSettings}>
        <ChannelFormConfig
          bind:this={formConfigRef}
          bind:formData={formChannelFormData}
        />
      </ChannelAdminSettings>
    {:else if tabId === 'integration'}
      <div class="px-16 py-8 max-w-3xl">
        {#if formChannelFormData.slug}
          <FormIntegrationPanel slug={formChannelFormData.slug} />
        {:else}
          <div class="text-center py-12">
            <IconCode class="w-12 h-12 mx-auto mb-3" style="color: var(--ds-text-subtle);" />
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              {t('channel.formSlugRequired', 'Set a form slug in Settings to enable integration options.')}
            </p>
          </div>
        {/if}
      </div>
    {:else if tabId === 'managers' && $isSystemAdmin}
      <div class="px-16 py-8">
        <ChannelManagersTab
          channelId={channel.id}
          channelName={channel.name}
          isDefault={channel.is_default}
        />
      </div>
    {/if}
  {/snippet}

  {#snippet after()}
    <CreateFormModal
      bind:isOpen={showCreateModal}
      channelId={channelId}
      onPrepare={prepareFormCreation}
      onCreated={handleFormCreated}
      onClose={() => showCreateModal = false}
    />
  {/snippet}
</ChannelAdminShell>
