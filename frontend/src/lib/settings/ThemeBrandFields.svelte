<script>
  import { t } from '../stores/i18n.svelte.js';
  import ButtonGroup from '../components/ButtonGroup.svelte';
  import LogoUploader from '../components/LogoUploader.svelte';

  // Brand layout + full/wordmark logo fields, shared by the theme create and
  // edit forms. The mark logos stay in the parent; this owns only the layout
  // choice and the full logo slots.
  let {
    brandLayout = $bindable('mark_text'),
    logoFullUrl = $bindable(''),
    logoFullUrlDark = $bindable(''),
    uploading = false,
    onUpload = () => {},
    idPrefix = 'theme',
  } = $props();

  const layouts = [
    {
      value: 'mark_text',
      labelKey: 'settings.themeManager.brandLayoutMarkText',
      fallback: 'Mark + text',
    },
    {
      value: 'mark_only',
      labelKey: 'settings.themeManager.brandLayoutMarkOnly',
      fallback: 'Mark only',
    },
    {
      value: 'full_logo',
      labelKey: 'settings.themeManager.brandLayoutFullLogo',
      fallback: 'Full logo',
    },
  ];
</script>

<div class="space-y-4">
  <div>
    <span class="block text-xs font-medium mb-1.5 text-ds-text">
      {t('settings.themeManager.brandLayout', 'Sidebar brand')}
    </span>
    <ButtonGroup
      ariaLabel={t('settings.themeManager.brandLayout', 'Sidebar brand')}
      testid={`${idPrefix}-brand-layout`}
    >
      {#snippet children(segment)}
        {#each layouts as layout (layout.value)}
          <button
            type="button"
            class={segment}
            data-active={brandLayout === layout.value}
            data-testid={`${idPrefix}-brand-layout-${layout.value}`}
            onclick={() => (brandLayout = layout.value)}
          >
            {t(layout.labelKey, layout.fallback)}
          </button>
        {/each}
      {/snippet}
    </ButtonGroup>
    <p class="text-xs mt-2 text-ds-text-subtle">
      {t(
        'settings.themeManager.brandLayoutHelp',
        'Controls the expanded sidebar. The collapsed sidebar always shows the mark.'
      )}
    </p>
  </div>

  {#if brandLayout === 'full_logo'}
    <LogoUploader
      currentLogoUrl={logoFullUrl || null}
      accept="image/png,image/jpeg,image/svg+xml,image/gif,image/webp"
      onUpload={(files) => onUpload(files, 'logo_full_url')}
      onRemove={() => (logoFullUrl = '')}
      {uploading}
      maxHeight="40px"
      label={t('lookAndFeel.fullLogoLight', 'Full Logo (Light Mode)')}
      helpText={t(
        'lookAndFeel.fullLogoHelp',
        'Wide or wordmark logo used in the expanded sidebar when the layout is "Full logo".'
      )}
    />

    <LogoUploader
      currentLogoUrl={logoFullUrlDark || null}
      accept="image/png,image/jpeg,image/svg+xml,image/gif,image/webp"
      onUpload={(files) => onUpload(files, 'logo_full_url_dark')}
      onRemove={() => (logoFullUrlDark = '')}
      {uploading}
      maxHeight="40px"
      label={t('lookAndFeel.fullLogoDark', 'Full Logo (Dark Mode)')}
      helpText={t(
        'lookAndFeel.darkLogoFallback',
        'Shown in dark mode. Falls back to the light mode logo when left empty.'
      )}
    />
  {/if}
</div>
