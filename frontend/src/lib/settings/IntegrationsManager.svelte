<script>
	// OAuth integrations share a page: outbound providers connect Windshift to
	// external apps, while inbound clients authorize apps to mint user tokens.

	import { onMount } from 'svelte';
	import Tabs from '../components/Tabs.svelte';
	import IntegrationProviderManager from './IntegrationProviderManager.svelte';
	import OAuthClientManager from './OAuthClientManager.svelte';
	import ZammadConnectionManager from './ZammadConnectionManager.svelte';
	import { ArrowUpRight, ArrowDownLeft, TicketCheck } from '@lucide/svelte';
	import { t } from '../stores/i18n.svelte.js';

	let activeTab = $state('outbound');

	onMount(() => {
		if (new URLSearchParams(window.location.search).get('tab') === 'zammad') {
			activeTab = 'zammad';
		}
	});

	const tabs = $derived([
		{ id: 'outbound', label: t('integrations.directions.outbound'), icon: ArrowUpRight },
		{ id: 'inbound', label: t('integrations.directions.inbound'), icon: ArrowDownLeft },
		{ id: 'zammad', label: t('zammad.tab'), icon: TicketCheck },
	]);
</script>

<div class="space-y-4">
	<Tabs {tabs} bind:activeTab>
		{#if activeTab === 'outbound'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				{t('integrations.directions.outboundDescription')}
			</p>
			<IntegrationProviderManager />
		{:else if activeTab === 'inbound'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				{t('integrations.directions.inboundDescription')}
			</p>
			<OAuthClientManager />
		{:else if activeTab === 'zammad'}
			<ZammadConnectionManager />
		{/if}
	</Tabs>
</div>
