<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import Select from '../../components/Select.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';

  let {
    formData = $bindable({
      slug: '',
      workspace_ids: [],
      enabled: false,
      title: '',
      description: '',
      registration_mode: 'open',
      allowed_domains: ''
    })
  } = $props();

  function parseDomains(input) {
    if (!input) return [];
    return String(input)
      .split(/[\s,;]+/)
      .map((d) => d.trim().toLowerCase())
      .filter(Boolean);
  }

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
      portal_title: formData.title || formData.slug,
      portal_description: formData.description || '',
      portal_registration_mode: formData.registration_mode === 'manual' ? 'manual' : 'open',
      portal_allowed_domains: parseDomains(formData.allowed_domains)
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
      <DescriptionText>
        {t('channel.portalUrl')}: /portal/{formData.slug || 'your-slug'}
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
      <Label color="default" class="mb-2">{t('channel.portalTitle')}</Label>
      <Input bind:value={formData.title} placeholder="Support Portal" />
    </div>

    <div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">
        {t('channel.portalRegistration', 'Customer Access')}
      </h5>

      <div class="space-y-4">
        <div>
          <Label color="default" class="mb-2">
            {t('channel.portalRegistrationMode', 'Registration Mode')}
          </Label>
          <Select
            bind:value={formData.registration_mode}
            options={[
              { value: 'open', label: t('channel.portalRegistrationOpen', 'Open — anyone with an allowed email can sign in') },
              { value: 'manual', label: t('channel.portalRegistrationManual', 'Manual — only admin-managed customers can sign in') }
            ]}
          />
          <DescriptionText>
            {formData.registration_mode === 'manual'
              ? t('channel.portalRegistrationManualHelp', 'Only portal customers you have added and granted channel access can request a magic link.')
              : t('channel.portalRegistrationOpenHelp', 'Any visitor whose email matches the domain allow-list (or any visitor, if the list is empty) can self-register.')}
          </DescriptionText>
        </div>

        <div>
          <Label color="default" class="mb-2">
            {t('channel.portalAllowedDomains', 'Allowed Email Domains')}
          </Label>
          <Input
            bind:value={formData.allowed_domains}
            placeholder="acme.com, partner.io"
          />
          <DescriptionText>
            {t('channel.portalAllowedDomainsHelp', 'Comma-separated list. Leave empty to allow any domain. Applies to both open and manual registration.')}
          </DescriptionText>
        </div>
      </div>
    </div>

    <!-- Enable Portal Toggle - Prominent -->
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
            {t('channel.enablePortal')}
          </div>
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            {formData.enabled ? t('channel.portalIsActive', 'Portal is active and accepting submissions') : t('channel.portalIsInactive', 'Portal is currently disabled')}
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
