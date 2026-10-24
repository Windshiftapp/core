<script>
  // Customer context for external requests (WI-1139): requester, organisation,
  // and the organisation's other requests plus linked assets. It reuses the
  // existing requester/organisation/linked-asset endpoints and only loads when
  // the agent expands the section, so internal items never trigger the calls.
  import { onDestroy } from 'svelte';
  import Building2 from '@lucide/svelte/icons/building-2';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Package from '@lucide/svelte/icons/package';
  import User from '@lucide/svelte/icons/user';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Text from '../../components/Text.svelte';
  import ItemKey from '../../components/ItemKey.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { customerOrganisations } from '../../api/portal.js';
  import { itemLinkedAssets } from '../../api/assets.js';
  import { itemUrl } from '../../utils/urls.js';

  let { item } = $props();

  let expanded = $state(false);
  let loading = $state(false);
  let loaded = $state(false);
  let organisationTickets = $state([]);
  let assets = $state([]);

  const controller = new AbortController();
  onDestroy(() => controller.abort());

  const organisationId = $derived(item?.creator_customer_organisation_id ?? null);
  const requesterName = $derived(
    item?.creator_portal_customer_name || item?.creator_name || ''
  );
  const requesterEmail = $derived(
    item?.creator_portal_customer_email || item?.creator_email || ''
  );

  async function load() {
    if (loaded || loading) return;
    loading = true;
    // A missing organisation or a denied org ACL is not an error: the section
    // still shows the requester and any linked assets.
    const [orgResult, assetResult] = await Promise.all([
      organisationId
        ? customerOrganisations
            .getTickets(organisationId, { signal: controller.signal })
            .catch(() => [])
        : Promise.resolve([]),
      itemLinkedAssets
        .get(item.id, { signal: controller.signal })
        .catch(() => []),
    ]);
    organisationTickets = Array.isArray(orgResult) ? orgResult : [];
    assets = Array.isArray(assetResult) ? assetResult : [];
    loading = false;
    loaded = true;
  }

  function toggle() {
    expanded = !expanded;
    if (expanded) load();
  }
</script>

<div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);">
  <button
    type="button"
    class="w-full flex items-center justify-between gap-2 group"
    aria-expanded={expanded}
    data-testid="customer-context-toggle"
    onclick={toggle}
  >
    <span class="flex items-center gap-2">
      <Building2 class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">
        {t('items.customerContext')}
      </Text>
    </span>
    <ChevronRight
      class="w-4 h-4 flex-shrink-0 transition-transform {expanded ? 'rotate-90' : ''}"
      style="color: var(--ds-text-subtle);"
    />
  </button>

  {#if expanded}
    {#if loading}
      <div class="pt-3 flex justify-center">
        <Spinner size="small" />
      </div>
    {:else}
      <div class="mt-3 space-y-3" data-testid="customer-context">
        <div class="flex items-start gap-2" data-testid="customer-context-requester">
          <User class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <div class="min-w-0">
            <Text variant="subtle" size="xs" weight="medium">
              {t('items.customerContextRequester')}
            </Text>
            <div class="text-sm truncate" style="color: var(--ds-text);">
              {requesterName || requesterEmail || t('common.unknown')}
            </div>
            {#if requesterName && requesterEmail}
              <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                {requesterEmail}
              </div>
            {/if}
          </div>
        </div>

        {#if item?.creator_customer_organisation_name}
          <div class="flex items-start gap-2" data-testid="customer-context-organisation">
            <Building2 class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
            <div class="min-w-0">
              <Text variant="subtle" size="xs" weight="medium">
                {t('items.customerContextOrganisation')}
              </Text>
              <div class="text-sm truncate" style="color: var(--ds-text);">
                {item.creator_customer_organisation_name}
              </div>
            </div>
          </div>
        {/if}

        {#if organisationId}
          <div data-testid="customer-context-org-tickets">
            <Text variant="subtle" size="xs" weight="medium">
              {t('items.customerContextOrgTickets')}
            </Text>
            {#if organisationTickets.length === 0}
              <div class="mt-1">
                <Text variant="subtle" size="sm">{t('items.customerContextOrgTicketsEmpty')}</Text>
              </div>
            {:else}
              <div class="mt-2 space-y-2">
                {#each organisationTickets as ticket (ticket.id)}
                  <a
                    href={itemUrl({ workspaceId: ticket.workspace_id, itemId: ticket.id })}
                    data-testid={`customer-context-org-ticket-${ticket.id}`}
                    class="block p-2.5 rounded border transition-colors hover:opacity-90"
                    style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
                  >
                    <div class="flex items-center justify-between gap-2">
                      <ItemKey item={ticket} clickable={false} />
                      {#if ticket.status_name}
                        <Lozenge color="gray">{ticket.status_name}</Lozenge>
                      {/if}
                    </div>
                    <div class="mt-1 text-sm truncate" style="color: var(--ds-text);">
                      {ticket.title}
                    </div>
                    {#if ticket.creator_contact_name || ticket.creator_contact_email}
                      <div class="mt-0.5 text-xs truncate" style="color: var(--ds-text-subtle);">
                        {ticket.creator_contact_name || ticket.creator_contact_email}
                      </div>
                    {/if}
                  </a>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <div class="flex items-start gap-2" data-testid="customer-context-assets">
          <Package class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <div class="min-w-0 flex-1">
            <Text variant="subtle" size="xs" weight="medium">
              {t('items.customerContextAssets')}
            </Text>
            {#if assets.length === 0}
              <div class="mt-1">
                <Text variant="subtle" size="sm">{t('items.customerContextAssetsEmpty')}</Text>
              </div>
            {:else}
              <div class="mt-2 space-y-1">
                {#each assets as asset (asset.link_id)}
                  <div
                    class="px-2 py-1.5 rounded border"
                    style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
                    data-testid={`customer-context-asset-${asset.id}`}
                  >
                    <div class="text-sm truncate" style="color: var(--ds-text);">{asset.title}</div>
                    {#if asset.type_name || asset.set_name}
                      <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                        {asset.type_name || asset.set_name}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>
