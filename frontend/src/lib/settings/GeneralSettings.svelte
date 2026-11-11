<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import IconSettings from '@tabler/icons-svelte-runes/icons/settings';
  import Button from '../components/Button.svelte';
  import Panel from '../components/Panel.svelte';
  import TextField from '../components/TextField.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { generalSettings } from '../api/admin.js';
  import { brandStore } from '../stores/brand.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { successToast, surfaceError } from '../stores/toasts.svelte.js';

  let loading = $state(true);
  let saving = $state(false);
  let siteTitle = $state('');

  onMount(async () => {
    try {
      const settings = await generalSettings.get();
      siteTitle = settings.site_title || '';
    } catch (err) {
      surfaceError(err, { context: 'loadGeneralSettings' });
    } finally {
      loading = false;
    }
  });

  async function save() {
    saving = true;
    try {
      const updated = await generalSettings.update({ site_title: siteTitle });
      siteTitle = updated.site_title || '';
      await brandStore.reload();
      if (typeof document !== 'undefined') {
        document.title = brandStore.siteTitle;
      }
      successToast(t('settings.generalSettings.saved', 'General settings saved'));
    } catch (err) {
      surfaceError(err, { context: 'saveGeneralSettings' });
    } finally {
      saving = false;
    }
  }
</script>

<div class="general-settings">
  <PageHeader
    icon={IconSettings}
    title={t('settings.generalSettings.title', 'General')}
    description={t('settings.generalSettings.subtitle', 'Instance-wide configuration')}
  />

  {#if loading}
    <StateDisplay type="loading" />
  {:else}
    <Panel class="max-w-2xl">
      <h2 class="text-base font-semibold mb-1 text-ds-text">
        {t('settings.generalSettings.siteIdentity', 'Site identity')}
      </h2>
      <p class="text-sm mb-4 text-ds-text-subtle">
        {t(
          'settings.generalSettings.siteIdentityDescription',
          'The product name shown in the browser tab, the sidebar, and on the login and setup screens.'
        )}
      </p>

      <TextField
        label={t('settings.generalSettings.siteTitle', 'Site title')}
        id="general-site-title"
        type="text"
        bind:value={siteTitle}
        placeholder="Windshift"
        maxlength={60}
        dataTestid="general-site-title"
      />

      <div
        class="mt-4 rounded-md border border-ds-border bg-ds-surface p-4"
      >
        <div class="text-xs mb-2 text-ds-text-subtle">
          {t('settings.generalSettings.preview', 'Preview')}
        </div>
        <div class="flex items-center gap-2">
          <img src="windshift-3.svg" alt="" class="w-6 h-6" />
          <span class="font-semibold text-sm text-ds-text">
            {siteTitle || 'Windshift'}
          </span>
        </div>
      </div>

      <div class="mt-4 flex justify-end">
        <Button
          variant="primary"
          onclick={save}
          disabled={saving}
          dataTestid="general-settings-save"
        >
          {saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
        </Button>
      </div>
    </Panel>
  {/if}
</div>
