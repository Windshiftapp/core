<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
	import TextField from '../../components/TextField.svelte';
	import TextareaField from '../../components/TextareaField.svelte';

  let {
    formData = $bindable({
      slug: '',
      workspace_ids: [],
      enabled: false,
      theme: 'light',
      brand_color: '#14b8a6',
      logo_url: '',
      success_message: '',
      redirect_url: ''
    }),
  } = $props();

  const themes = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'auto', label: 'Auto' }
  ];

  const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$/;
  const HEX_COLOR_PATTERN = /^#[0-9a-fA-F]{6}$/;

  function isValidHttpUrl(value) {
    if (!value) return true;
    try {
      const url = new URL(value);
      return url.protocol === 'http:' || url.protocol === 'https:';
    } catch {
      return false;
    }
  }

  export function validate() {
    if (!formData.slug?.trim()) {
      return { valid: false, message: t('channel.formSlugRequired') };
    }
    if (!SLUG_PATTERN.test(formData.slug.trim())) {
      return { valid: false, message: t('channel.formSlugInvalid') };
    }
    if (formData.brand_color && !HEX_COLOR_PATTERN.test(formData.brand_color)) {
      return { valid: false, message: t('channel.formBrandColorInvalid') };
    }
    if (!isValidHttpUrl(formData.logo_url)) {
      return { valid: false, message: t('channel.formLogoUrlInvalid') };
    }
    if (!isValidHttpUrl(formData.redirect_url)) {
      return { valid: false, message: t('channel.formRedirectUrlInvalid') };
    }
    return { valid: true };
  }

  export function getConfig() {
    return {
      form_slug: formData.slug.trim(),
      form_workspace_ids: formData.workspace_ids,
      form_theme: formData.theme || 'light',
      form_brand_color: formData.brand_color || '#14b8a6',
      form_logo_url: formData.logo_url || '',
      form_success_message: formData.success_message || '',
      form_redirect_url: formData.redirect_url || ''
    };
  }
</script>

<div class="pt-6 border-t" style="border-color: var(--ds-border);">
  <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.formConfiguration')}</h4>
  <DescriptionText>
    Set the shared public URL and optional appearance for this channel.
  </DescriptionText>

  <div class="mt-4 space-y-4">
    <div>
      {#snippet formSlugLabel()}
        {t('channel.formSlug')} <span class="text-xs font-normal" style="color: var(--ds-text-subtle);">({t('channel.formSlugHelp')})</span>
      {/snippet}
      <TextField
        labelSnippet={formSlugLabel}
        labelColor="default"
        labelClass="mb-2"
        required
        placeholder="contact-form"
        pattern="[a-z0-9\-]+"
        title={t('validation.slugInvalid')}
        bind:value={formData.slug}
      />
      <DescriptionText>
        Forms will be shared at /forms/{formData.slug || 'your-url'}.
      </DescriptionText>
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.formTheme')}</Label>
      <div class="flex gap-2">
        {#each themes as theme}
          <button
            type="button"
            class="px-4 py-2 rounded-lg text-sm font-medium border transition-colors"
            style="background-color: {formData.theme === theme.value ? 'var(--ds-background-selected)' : 'var(--ds-surface)'}; border-color: {formData.theme === theme.value ? 'var(--ds-border-selected)' : 'var(--ds-border)'}; color: var(--ds-text);"
            onclick={() => formData.theme = theme.value}
          >
            {theme.label}
          </button>
        {/each}
      </div>
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.formBrandColor')}</Label>
      <div class="flex items-center gap-3">
        <input
          type="color"
          bind:value={formData.brand_color}
          class="w-10 h-10 rounded border cursor-pointer"
          style="border-color: var(--ds-border);"
        />
        <Input bind:value={formData.brand_color} placeholder="#14b8a6" class="flex-1" />
      </div>
    </div>

    <div>
      <TextField
        label={t('channel.formLogoUrl')}
        labelColor="default"
        placeholder="https://example.com/logo.png"
        bind:value={formData.logo_url}
      />
    </div>

    <div>
      <TextareaField
        label={t('channel.formSuccessMessage')}
        labelColor="default"
        placeholder={t('channel.formSuccessMessagePlaceholder')}
        rows={2}
        bind:value={formData.success_message}
      />
    </div>

    <div>
      <TextField
        label={t('channel.formRedirectUrl')}
        labelColor="default"
        placeholder="https://example.com/thank-you"
        bind:value={formData.redirect_url}
      />
      <DescriptionText>
        {t('channel.formRedirectUrlHelp')}
      </DescriptionText>
    </div>

  </div>
</div>
