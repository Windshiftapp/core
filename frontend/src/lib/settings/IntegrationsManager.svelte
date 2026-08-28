<script>
	// OAuth integrations share a page: outbound providers connect Windshift to
	// external apps, while inbound clients authorize apps to mint user tokens.

	import Tabs from '../components/Tabs.svelte';
	import IntegrationProviderManager from './IntegrationProviderManager.svelte';
	import OAuthClientManager from './OAuthClientManager.svelte';
	import { ArrowUpRight, ArrowDownLeft } from '@lucide/svelte';
	import { t } from '../stores/i18n.svelte.js';

	let activeTab = $state('outbound');

	const tabs = $derived([
		{ id: 'outbound', label: t('integrations.directions.outbound'), icon: ArrowUpRight },
		{ id: 'inbound', label: t('integrations.directions.inbound'), icon: ArrowDownLeft },
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
		{/if}
	</Tabs>
</div>
