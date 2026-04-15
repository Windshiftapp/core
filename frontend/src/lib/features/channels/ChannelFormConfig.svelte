<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import FormIntegrationPanel from './FormIntegrationPanel.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';

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
    })
  } = $props();

  const themes = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'auto', label: 'Auto' }
  ];

  export function validate() {
    if (!formData.slug?.trim()) {
      return { valid: false, message: t('channel.formSlugRequired') };
    }
    if (!formData.workspace_ids?.length) {
      return { valid: false, message: t('channel.selectAtLeastOneWorkspace') };
    }
    return { valid: true };
  }

  export function getConfig() {
    return {
      form_slug: formData.slug,
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

  <div class="space-y-4">
    <div>
      <Label color="default" required class="mb-2">
        {t('channel.formSlug')} <span class="text-xs font-normal" style="color: var(--ds-text-subtle);">({t('channel.formSlugHelp')})</span>
      </Label>
      <Input
        bind:value={formData.slug}
        required
        placeholder="contact-form"
        pattern="[a-z0-9\-]+"
        title={t('validation.slugInvalid')}
      />
      <DescriptionText>
        {t('channel.formUrl')}: /forms/{formData.slug || 'your-slug'}
      </DescriptionText>
    </div>

    <div>
      <WorkspacePicker
        bind:value={formData.workspace_ids}
        label="{t('channel.targetWorkspaces')} *"
        placeholder={t('channel.searchWorkspaces')}
      />
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
      <Label color="default" class="mb-2">{t('channel.formLogoUrl')}</Label>
      <Input bind:value={formData.logo_url} placeholder="https://example.com/logo.png" />
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.formSuccessMessage')}</Label>
      <Textarea
        bind:value={formData.success_message}
        placeholder={t('channel.formSuccessMessagePlaceholder')}
        rows={2}
      />
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.formRedirectUrl')}</Label>
      <Input bind:value={formData.redirect_url} placeholder="https://example.com/thank-you" />
      <DescriptionText>
        {t('channel.formRedirectUrlHelp')}
      </DescriptionText>
    </div>

    <!-- Integration Panel (only show when slug is set) -->
    {#if formData.slug}
      <div class="pt-4 border-t" style="border-color: var(--ds-border);">
        <FormIntegrationPanel slug={formData.slug} />
      </div>
    {/if}

    <!-- Enable Form Channel Toggle -->
    <div
      class="flex items-center justify-between p-4 rounded-lg border-2 transition-colors cursor-pointer"
      style="background-color: {formData.enabled ? 'var(--ds-background-success)' : 'var(--ds-surface-raised)'}; border-color: {formData.enabled ? 'var(--ds-border-success)' : 'var(--ds-border)'};"
      onclick={() => formData.enabled = !formData.enabled}
      role="button"
      tabindex="0"
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); formData.enabled = !formData.enabled; }}}
    >
      <div class="flex items-center gap-3">
        <div
          class="w-10 h-6 rounded-full relative transition-colors"
          style="background-color: {formData.enabled ? 'var(--ds-background-success-bold)' : 'var(--ds-background-neutral)'};"
        >
          <div
            class="absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform"
            style="transform: translateX({formData.enabled ? '22px' : '4px'});"
          ></div>
        </div>
        <div>
          <div class="text-sm font-semibold" style="color: var(--ds-text);">
            {t('channel.enableForm')}
          </div>
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            {formData.enabled ? t('channel.formIsActive') : t('channel.formIsInactive')}
          </div>
        </div>
      </div>
      <div
        class="px-3 py-1 rounded-full text-xs font-semibold"
        style="background-color: {formData.enabled ? 'var(--ds-background-success-bold)' : 'var(--ds-background-neutral)'}; color: {formData.enabled ? 'white' : 'var(--ds-text-subtle)'};"
      >
        {formData.enabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
      </div>
    </div>
  </div>
</div>
