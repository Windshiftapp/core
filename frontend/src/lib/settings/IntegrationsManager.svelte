<script>
	// Unified admin page for both directions of OAuth-shaped integrations.
	//
	// Outbound: Windshift acts as the OAuth *client* against external apps
	//           (Notion, Confluence, etc). Backed by `integration_providers`.
	//
	// Inbound:  Windshift acts as the OAuth *server*; third-party apps
	//           authorize Windshift users to mint per-user API tokens.
	//           Backed by `oauth_clients`.
	//
	// They model the same protocol from opposite sides, so they live in one
	// place but get distinct tabs (and distinct managers underneath) so the
	// admin form for each stays tight to its concept.

	import Tabs from '../components/Tabs.svelte';
	import IntegrationProviderManager from './IntegrationProviderManager.svelte';
	import OAuthClientManager from './OAuthClientManager.svelte';
	import { ArrowUpRight, ArrowDownLeft } from '@lucide/svelte';

	let activeTab = $state('outbound');

	const tabs = [
		{ id: 'outbound', label: 'Outbound', icon: ArrowUpRight },
		{ id: 'inbound', label: 'Inbound', icon: ArrowDownLeft },
	];
</script>

<div class="space-y-4">
	<Tabs {tabs} bind:activeTab>
		{#if activeTab === 'outbound'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				Apps that <strong>Windshift connects to</strong>. Add the OAuth credentials Windshift uses to read data from external services like Notion or Confluence.
			</p>
			<IntegrationProviderManager />
		{:else if activeTab === 'inbound'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				Apps that <strong>connect to Windshift</strong> on a user's behalf. Register a third-party app once; users then authorize it via OAuth 2.0 (authorization code + PKCE) to mint per-user Windshift API tokens.
			</p>
			<OAuthClientManager />
		{/if}
	</Tabs>
</div>
