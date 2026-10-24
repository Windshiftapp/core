<script>
  import IconExternalLink from '@tabler/icons-svelte-runes/icons/external-link';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import TabStrip from '../../components/TabStrip.svelte';

  let {
    loading = false,
    channel = null,
    activeTab = $bindable('settings'),
    tabs = [],
    subtitle = '',
    openUrl = '',
    openLabel = '',
    children,
    after = undefined,
  } = $props();

  let stripTabs = $derived(
    tabs.map((tab) => ({
      id: tab.id,
      label: tab.label(),
      icon: tab.icon,
      testid: `channel-tab-${tab.id}`,
    })),
  );
</script>

{#if loading}
  <div class="flex-1 flex items-center justify-center py-24">
    <Spinner />
  </div>
{:else if channel}
  <div class="flex-1 flex flex-col overflow-hidden">
    <div class="px-16 pt-8 pb-0">
      <div class="mb-4">
        <BackButton href="/admin/channels" label={t('channels.title')} />
      </div>

      <div class="flex items-center justify-between mb-6">
        <div>
          <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
            {channel.name}
          </h1>
          <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
            {subtitle}
          </p>
        </div>
        {#if openUrl}
          <Button
            onclick={() => window.open(openUrl, '_blank')}
            variant="default"
            size="small"
            icon={IconExternalLink}
          >
            {openLabel}
          </Button>
        {/if}
      </div>

      <TabStrip tabs={stripTabs} bind:activeTab />
    </div>

    <div class="flex-1 overflow-y-auto">
      {@render children?.(activeTab)}
    </div>
  </div>

  {@render after?.()}
{/if}
