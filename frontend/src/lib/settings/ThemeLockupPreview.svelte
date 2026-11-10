<script>
  import { resolveBrandLockup } from '../stores/brand.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  // Live preview of the sidebar brand lockup for the theme currently being
  // edited. Mirrors MainSidebar's resolveBrandLockup so the layout choice and
  // the uploaded logos are shown exactly as the sidebar will render them.
  let {
    siteTitle = '',
    brandLayout = 'mark_text',
    logoUrl = '',
    logoUrlDark = '',
    logoFullUrl = '',
    logoFullUrlDark = '',
    lightBackground = '#ffffff',
    lightText = '#374151',
    darkBackground = '#1f2937',
    darkText = '#f3f4f6',
  } = $props();

  const snapshot = $derived({
    siteTitle,
    brandLayout,
    logoURL: logoUrl,
    logoURLDark: logoUrlDark,
    logoFullURL: logoFullUrl,
    logoFullURLDark: logoFullUrlDark,
  });

  const variants = $derived([
    { key: 'light-expanded', expanded: true, background: lightBackground, color: lightText },
    { key: 'light-collapsed', expanded: false, background: lightBackground, color: lightText },
    { key: 'dark-expanded', expanded: true, background: darkBackground, color: darkText },
    { key: 'dark-collapsed', expanded: false, background: darkBackground, color: darkText },
  ].map((variant) => ({
    ...variant,
    lockup: resolveBrandLockup({ isDark: variant.key.startsWith('dark'), expanded: variant.expanded }, snapshot),
  })));
</script>

<div class="space-y-2">
  <span class="block text-xs font-medium text-ds-text">
    {t('settings.themeManager.brandPreview', 'Sidebar preview')}
  </span>
  <div class="grid grid-cols-2 gap-3">
    {#each variants as variant (variant.key)}
      <div>
        <span class="block text-[10px] uppercase tracking-wide mb-1 text-ds-text-subtle">
          {variant.key.startsWith('light')
            ? t('settings.lightMode', 'Light')
            : t('settings.darkMode', 'Dark')}
          ·
          {variant.expanded
            ? t('settings.themeManager.brandPreviewExpanded', 'Expanded')
            : t('settings.themeManager.brandPreviewCollapsed', 'Collapsed')}
        </span>
        <div
          class="flex h-10 items-center overflow-hidden rounded border border-ds-border px-3"
          style="background-color: {variant.background}; color: {variant.color};"
          data-testid={`theme-brand-preview-${variant.key}`}
        >
          <img
            src={variant.lockup.src || 'windshift-3.svg'}
            alt=""
            class={variant.lockup.variant === 'full'
              ? 'max-h-6 max-w-full object-contain flex-shrink-0'
              : 'max-w-6 max-h-6 object-contain flex-shrink-0'}
          />
          {#if variant.lockup.showText}
            <span class="ml-2 truncate text-xs font-semibold">{variant.lockup.text}</span>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
