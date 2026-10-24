<script>
	import { onMount } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { setOAuthReturnURL } from '../utils/oauthReturn.js';
	import { scmProviders, userSCM } from '../api/scm.js';
	import { userIntegrations } from '../api/integrations.js';
 import GitBranch from '@lucide/svelte/icons/git-branch';
 import CheckCircle from '@lucide/svelte/icons/check-circle';
 import XCircle from '@lucide/svelte/icons/x-circle';
 import LogOut from '@lucide/svelte/icons/log-out';
 import ExternalLink from '@lucide/svelte/icons/external-link';
	import Github from '@tabler/icons-svelte-runes/icons/brand-github';
	import Button from '../components/Button.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import TodoistSyncSettings from './TodoistSyncSettings.svelte';
	import { t } from '../stores/i18n.svelte.js';
	import { successToast } from '../stores/toasts.svelte.js';
	import { formatDateSimple } from '../utils/dateFormatter.js';

	let loading = $state(true);
	let providers = $state([]);
	let error = $state('');
	let disconnecting = $state(null);

	// Integration connections
	let intLoading = $state(true);
	let intConnections = $state([]);
	let intAvailable = $state([]);
	let intError = $state('');
	let intDisconnecting = $state(null);

	onMount(() => {
		// Landed back from the OAuth callback (via the app shell's return
		// navigation): acknowledge the outcome and clean the URL.
		const oauthParams = new URLSearchParams(window.location.search);
		if (oauthParams.get('oauth') === 'success') {
			successToast(t('settings.connectedAccounts.oauthConnected'));
			window.history.replaceState({}, '', window.location.pathname);
		} else if (oauthParams.get('oauth') === 'error') {
			error = oauthParams.get('message') || t('settings.connectedAccounts.oauthFailedCallback');
			window.history.replaceState({}, '', window.location.pathname);
		}
		loadProviders();
		loadIntegrations();
	});

	async function loadProviders() {
		loading = true;
		error = '';
		try {
			providers = await userSCM.getAvailableProviders() || [];
		} catch (err) {
			console.error('Failed to load SCM providers:', err);
			error = t('settings.connectedAccounts.failedToLoad');
			providers = [];
		} finally {
			loading = false;
		}
	}

	async function disconnect(providerId) {
		disconnecting = providerId;
		error = '';
		try {
			await userSCM.disconnect(providerId);
			// Refresh providers list
			await loadProviders();
		} catch (err) {
			console.error('Failed to disconnect:', err);
			error = t('settings.connectedAccounts.failedToDisconnect');
		} finally {
			disconnecting = null;
		}
	}

	function connect(provider) {
		// Start OAuth flow
		scmProviders.startOAuth(provider.slug).then(result => {
			if (result?.auth_url) {
				// Store return URL so we come back here
				setOAuthReturnURL();
				window.location.href = result.auth_url;
			}
		}).catch(err => {
			console.error('Failed to start OAuth:', err);
			error = t('settings.connectedAccounts.failedToStartConnection');
		});
	}

	function getProviderIcon(providerType) {
		switch (providerType?.toLowerCase()) {
			case 'github':
				return Github;
			default:
				return GitBranch;
		}
	}

	async function loadIntegrations() {
		intLoading = true;
		intError = '';
		try {
			const [connections, available] = await Promise.all([
				userIntegrations.getConnections(),
				userIntegrations.getAvailableProviders(),
			]);
			intConnections = connections || [];
			intAvailable = available || [];
		} catch (err) {
			console.error('Failed to load integrations:', err);
			intError = 'Failed to load integration connections';
		} finally {
			intLoading = false;
		}
	}

	async function disconnectIntegration(providerId) {
		intDisconnecting = providerId;
		try {
			await userIntegrations.disconnect(providerId);
			await loadIntegrations();
		} catch (err) {
			console.error('Failed to disconnect integration:', err);
			intError = t('integrations.failedToLoadLinks');
		} finally {
			intDisconnecting = null;
		}
	}

	function connectIntegration(provider) {
		userIntegrations.startOAuth(provider.slug).then(result => {
			if (result?.auth_url) {
				sessionStorage.setItem('integration_oauth_return', window.location.href);
				window.location.href = result.auth_url;
			}
		}).catch(err => {
			console.error('Failed to start integration OAuth:', err);
			intError = 'Failed to start connection';
		});
	}

	function getIntegrationIcon(providerType) {
		switch (providerType?.toLowerCase()) {
			case 'notion':
				return ExternalLink;
			default:
				return ExternalLink;
		}
	}

	function getConnectedProvider(providerId) {
		return intConnections.find(c => c.integration_provider_id === providerId);
	}

	function parseMetadata(metadataStr) {
		try {
			return JSON.parse(metadataStr || '{}');
		} catch {
			return {};
		}
	}
</script>

<div>
	{#if error}
		<div class="mb-4">
			<AlertBox message={error} />
		</div>
	{/if}

	{#if loading}
		<StateDisplay type="loading" />
	{:else if providers.length === 0}
		<EmptyState
			icon={GitBranch}
			title={t('settings.connectedAccounts.noProvidersTitle')}
			description={t('settings.connectedAccounts.noProvidersDesc')}
		/>
	{:else}
		<div class="space-y-4">
			{#each providers as provider}
				{@const ProviderIcon = getProviderIcon(provider.provider_type)}
				<div
					class="border rounded-lg p-4 flex items-center gap-4"
					style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
				>
					<!-- Provider Icon -->
					<div
						class="w-12 h-12 rounded-lg flex items-center justify-center"
						style="background-color: var(--ds-background-neutral);"
					>
						<ProviderIcon
							class="w-6 h-6"
							style="color: var(--ds-text);"
						/>
					</div>

					<!-- Provider Info -->
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<h3 class="text-base font-medium" style="color: var(--ds-text);">
								{provider.name}
							</h3>
							{#if provider.is_connected}
								<span
									class="flex items-center gap-1 text-xs px-2 py-0.5 rounded-full"
									style="background-color: var(--ds-background-success); color: var(--ds-text-success);"
								>
									<CheckCircle class="w-3 h-3" />
									{t('settings.connectedAccounts.connected')}
								</span>
							{:else}
								<span
									class="flex items-center gap-1 text-xs px-2 py-0.5 rounded-full"
									style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
								>
									<XCircle class="w-3 h-3" />
									{t('settings.connectedAccounts.notConnected')}
								</span>
							{/if}
						</div>

						{#if provider.is_connected && provider.scm_username}
							<div class="flex items-center gap-2 mt-1">
								{#if provider.scm_avatar_url}
									<img
										src={provider.scm_avatar_url}
										alt={provider.scm_username}
										class="w-5 h-5 rounded-full"
									/>
								{/if}
								<span class="text-sm" style="color: var(--ds-text-subtle);">
									@{provider.scm_username}
								</span>
								{#if provider.connected_at}
									<span class="text-xs" style="color: var(--ds-text-subtlest);">
										{t('settings.connectedAccounts.connectedOn')} {formatDateSimple(provider.connected_at)}
									</span>
								{/if}
							</div>
						{:else if !provider.is_connected}
							<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
								{t('settings.connectedAccounts.connectDesc')}
							</p>
						{/if}
					</div>

					<!-- Actions -->
					<div class="flex items-center gap-2">
						{#if provider.is_connected}
							<Button
								variant="danger"
								size="small"
								onclick={() => disconnect(provider.id)}
								loading={disconnecting === provider.id}
								icon={LogOut}
							>
								{disconnecting === provider.id ? t('settings.connectedAccounts.disconnecting') : t('settings.connectedAccounts.disconnect')}
							</Button>
						{:else}
							<Button
								variant="primary"
								size="small"
								onclick={() => connect(provider)}
							>
								{t('settings.connectedAccounts.connect')}
							</Button>
						{/if}
					</div>
				</div>
			{/each}
		</div>

		<!-- Info Text -->
		<div class="mt-6 text-xs" style="color: var(--ds-text-subtlest);">
			<p>
				{t('settings.connectedAccounts.footerNote')}
			</p>
		</div>
	{/if}

	<!-- Integration Connections Section -->
	{#if !intLoading && intAvailable.length > 0}
		<div class="mt-8">
			<h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">
				{t('integrations.title')}
			</h3>

			{#if intError}
				<div class="mb-4">
					<AlertBox message={intError} />
				</div>
			{/if}

			<div class="space-y-4">
				{#each intAvailable as provider}
					{@const connection = getConnectedProvider(provider.id)}
					{@const metadata = connection ? parseMetadata(connection.provider_metadata) : {}}
					{@const IntegrationIcon = getIntegrationIcon(provider.provider_type)}
					{@const isTodoist = provider.provider_type?.toLowerCase() === 'todoist'}
					<div
						class="border rounded-lg p-4"
						style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
					>
						<div class="flex items-center gap-4">
						<div
							class="w-12 h-12 rounded-lg flex items-center justify-center"
							style="background-color: var(--ds-background-neutral);"
						>
							<IntegrationIcon
								class="w-6 h-6"
								style="color: var(--ds-text);"
							/>
						</div>

						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2">
								<h3 class="text-base font-medium" style="color: var(--ds-text);">
									{provider.name}
								</h3>
								{#if connection}
									<span
										class="flex items-center gap-1 text-xs px-2 py-0.5 rounded-full"
										style="background-color: var(--ds-background-success); color: var(--ds-text-success);"
									>
										<CheckCircle class="w-3 h-3" />
										{t('integrations.connected')}
									</span>
								{:else}
									<span
										class="flex items-center gap-1 text-xs px-2 py-0.5 rounded-full"
										style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
									>
										<XCircle class="w-3 h-3" />
										{t('settings.connectedAccounts.notConnected')}
									</span>
								{/if}
							</div>

							{#if connection && metadata.workspace_name}
								<div class="flex items-center gap-2 mt-1 overflow-hidden">
									{#if metadata.workspace_icon}
										{#if metadata.workspace_icon.startsWith('http')}
											<img src={metadata.workspace_icon} alt="" class="w-4 h-4 rounded-sm shrink-0" />
										{:else}
											<span class="text-sm">{metadata.workspace_icon}</span>
										{/if}
									{/if}
									<span class="text-sm truncate" style="color: var(--ds-text-subtle);">
										{metadata.workspace_name}
									</span>
									{#if connection.connected_at}
										<span class="text-xs" style="color: var(--ds-text-subtlest);">
											{t('settings.connectedAccounts.connectedOn')} {formatDateSimple(connection.connected_at)}
										</span>
									{/if}
								</div>
							{:else if !connection}
								<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
									{t('integrations.connectToLink')}
								</p>
							{/if}
						</div>

						<div class="flex items-center gap-2">
							{#if connection}
								<Button
									variant="danger"
									size="small"
									onclick={() => disconnectIntegration(provider.id)}
									loading={intDisconnecting === provider.id}
									icon={LogOut}
								>
									{t('integrations.disconnect')}
								</Button>
							{:else}
								<Button
									variant="primary"
									size="small"
									onclick={() => connectIntegration(provider)}
								>
									{t('integrations.connect')}
								</Button>
							{/if}
						</div>
						</div>
						{#if isTodoist && connection}
							<TodoistSyncSettings />
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
