<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';

  let {
    formData = $bindable({
      slug: '',
      workspace_ids: [],
      enabled: false,
      title: '',
      description: ''
    })
  } = $props();

  export function validate() {
    if (!formData.slug?.trim()) {
      return { valid: false, message: t('channel.portalSlugRequired') };
    }
    if (!formData.workspace_ids?.length) {
      return { valid: false, message: t('channel.selectAtLeastOneWorkspace') };
    }
    return { valid: true };
  }

  export function getConfig() {
    return {
      portal_slug: formData.slug,
      portal_workspace_ids: formData.workspace_ids,
      portal_enabled: formData.enabled,
      portal_title: formData.title || formData.slug,
      portal_description: formData.description || ''
    };
  }
</script>

<div class="pt-6 border-t" style="border-color: var(--ds-border);">
  <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.portalConfiguration')}</h4>

  <div class="space-y-4">
    <div>
      <Label color="default" required class="mb-2">
        {t('channel.portalSlug')} <span class="text-xs font-normal" style="color: var(--ds-text-subtle);">({t('channel.portalSlugHelp')})</span>
      </Label>
      <Input
        bind:value={formData.slug}
        required
        placeholder="support-portal"
        pattern="[a-z0-9\-]+"
        title={t('validation.slugInvalid')}
      />
      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
        {t('channel.portalUrl')}: /portal/{formData.slug || 'your-slug'}
      </p>
    </div>

    <div>
      <WorkspacePicker
        bind:value={formData.workspace_ids}
        label="{t('channel.targetWorkspaces')} *"
        placeholder={t('channel.searchWorkspaces')}
      />
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.portalTitle')}</Label>
      <Input bind:value={formData.title} placeholder="Support Portal" />
    </div>

    <div>
      <Label color="default" class="mb-2">{t('channel.description')}</Label>
      <Textarea bind:value={formData.description} placeholder={t('channel.briefDescription')} rows={2} />
    </div>

    <div class="flex items-center gap-3 p-4 rounded" style="background-color: var(--ds-surface-raised);">
      <input type="checkbox" id="portalEnabled" bind:checked={formData.enabled} class="w-4 h-4 rounded" />
      <label for="portalEnabled" class="text-sm font-medium cursor-pointer" style="color: var(--ds-text);">
        {t('channel.enablePortal')}
      </label>
    </div>
  </div>
</div>
