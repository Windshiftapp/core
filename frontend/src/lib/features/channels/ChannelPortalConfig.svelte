<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import Select from '../../components/Select.svelte';
  import WorkspacePicker from '../../pickers/WorkspacePicker.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
	import TextField from '../../components/TextField.svelte';

  let {
    formData = $bindable({
      slug: '',
      workspace_ids: [],
      enabled: false,
      title: '',
      description: '',
      registration_mode: 'open',
      allowed_domains: ''
    }),
    connectedMailboxes = []
  } = $props();

  const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$/;
  const DOMAIN_PATTERN = /^(?!-)[a-z0-9-]+(?<!-)(?:\.(?!-)[a-z0-9-]+(?<!-))*\.[a-z]{2,}$/;

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
    if (!SLUG_PATTERN.test(formData.slug.trim())) {
      return { valid: false, message: t('channel.portalSlugInvalid') };
    }
    if (!formData.workspace_ids?.length) {
      return { valid: false, message: t('channel.selectAtLeastOneWorkspace') };
    }
    const domains = parseDomains(formData.allowed_domains);
    if (domains.some((d) => !DOMAIN_PATTERN.test(d))) {
      return { valid: false, message: t('channel.portalAllowedDomainsInvalid') };
    }
    return { valid: true };
  }

  export function getConfig() {
    return {
      portal_slug: formData.slug.trim(),
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
      {#snippet portalSlugLabel()}
        {t('channel.portalSlug')} <span class="text-xs font-normal" style="color: var(--ds-text-subtle);">({t('channel.portalSlugHelp')})</span>
      {/snippet}
      <TextField
        labelSnippet={portalSlugLabel}
        labelColor="default"
        labelClass="mb-2"
        required
        placeholder="support-portal"
        pattern="[a-z0-9\-]+"
        title={t('validation.slugInvalid')}
        bind:value={formData.slug}
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
      <TextField
        label={t('channel.portalTitle')}
        labelColor="default"
        placeholder="Support Portal"
        dataTestid="channel-portal-title"
        bind:value={formData.title}
      />
    </div>

    <div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">
        {t('channel.portalConnectedMailboxes', 'Connected Intake Mailboxes')}
      </h5>
      <div data-testid="portal-connected-mailboxes">
        {#if connectedMailboxes.length === 0}
          <DescriptionText>
            {t('channel.portalConnectedMailboxesEmpty', 'No intake mailboxes are connected to this portal.')}
          </DescriptionText>
        {:else}
          <ul class="space-y-2" data-testid="portal-connected-mailbox-list">
            {#each connectedMailboxes as mailbox (mailbox.id)}
              <li
                data-testid="portal-connected-mailbox-{mailbox.id}"
                class="p-3 rounded text-sm flex items-center justify-between gap-3"
                style="background: var(--ds-surface-raised); color: var(--ds-text);"
              >
                <span class="font-medium truncate">{mailbox.name}</span>
                <Lozenge color={mailbox.status === 'enabled' ? 'green' : 'gray'} dataTestid="portal-connected-mailbox-status-{mailbox.id}">
                  {mailbox.status === 'enabled'
                    ? t('channel.portalMailboxEnabled', 'Enabled')
                    : t('channel.portalMailboxDisabled', 'Disabled')}
                </Lozenge>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
      <DescriptionText>
        {t('channel.portalConnectedMailboxesHelp', "Email channels linked to this portal. Customers who email those addresses see their tickets here. Configure the link on each email channel.")}
      </DescriptionText>
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
          <TextField
            label={t('channel.portalAllowedDomains', 'Allowed Email Domains')}
            labelColor="default"
            labelClass="mb-2"
            placeholder="acme.com, partner.io"
            bind:value={formData.allowed_domains}
          />
          <DescriptionText>
            {t('channel.portalAllowedDomainsHelp', 'Comma-separated list. Leave empty to allow any domain. Applies to both open and manual registration.')}
          </DescriptionText>
        </div>
      </div>
    </div>

    <div class="flex items-center justify-between">
      <div>
        <div class="text-sm font-medium" style="color: var(--ds-text);">
          {t('channel.enablePortal')}
        </div>
        <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {formData.enabled
            ? t('channel.portalIsActive', 'Portal is active and accepting submissions')
            : t('channel.portalIsInactive', 'Portal is currently disabled')}
        </div>
      </div>
      <Toggle bind:checked={formData.enabled} />
    </div>
  </div>
</div>
