<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Github, GitBranch, CheckCircle, XCircle, LogOut, Loader2, ExternalLink } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import { t } from '../stores/i18n.svelte.js';

	let loading = $state(true);
	let providers = $state([]);
	let error = $state('');
	let disconnecting = $state(null);

	onMount(() => {
		loadProviders();
	});

	async function loadProviders() {
		loading = true;
		error = '';
		try {
			providers = await api.userSCM.getAvailableProviders() || [];
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
			await api.userSCM.disconnect(providerId);
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
		api.scmProviders.startOAuth(provider.slug).then(result => {
			if (result?.auth_url) {
				// Store return URL so we come back here
				sessionStorage.setItem('scm_oauth_return', window.location.href);
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
</script>

<div>
	{#if error}
		<div class="mb-4">
			<AlertBox message={error} />
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-8">
			<Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
		</div>
	{:else if providers.length === 0}
		<div class="text-center py-8">
			<GitBranch class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
			<h3 class="text-base font-medium mb-2" style="color: var(--ds-text);">{t('settings.connectedAccounts.noProvidersTitle')}</h3>
			<p class="text-sm" style="color: var(--ds-text-subtle);">
				{t('settings.connectedAccounts.noProvidersDesc')}
			</p>
		</div>
	{:else}
		<div class="space-y-4">
			{#each providers as provider}
				<div
					class="border rounded-lg p-4 flex items-center gap-4"
					style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
				>
					<!-- Provider Icon -->
					<div
						class="w-12 h-12 rounded-lg flex items-center justify-center"
						style="background-color: var(--ds-background-neutral);"
					>
						<svelte:component
							this={getProviderIcon(provider.provider_type)}
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
							{#if provider.connected}
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

						{#if provider.connected && provider.username}
							<div class="flex items-center gap-2 mt-1">
								{#if provider.avatar_url}
									<img
										src={provider.avatar_url}
										alt={provider.username}
										class="w-5 h-5 rounded-full"
									/>
								{/if}
								<span class="text-sm" style="color: var(--ds-text-subtle);">
									@{provider.username}
								</span>
								{#if provider.connected_at}
									<span class="text-xs" style="color: var(--ds-text-subtlest);">
										{t('settings.connectedAccounts.connectedOn')} {new Date(provider.connected_at).toLocaleDateString()}
									</span>
								{/if}
							</div>
						{:else if !provider.connected}
							<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
								{t('settings.connectedAccounts.connectDesc')}
							</p>
						{/if}
					</div>

					<!-- Actions -->
					<div class="flex items-center gap-2">
						{#if provider.connected}
							<Button
								variant="danger"
								size="small"
								onclick={() => disconnect(provider.id)}
								disabled={disconnecting === provider.id}
							>
								{#if disconnecting === provider.id}
									<Loader2 class="w-4 h-4 animate-spin mr-1" />
									{t('settings.connectedAccounts.disconnecting')}
								{:else}
									<LogOut class="w-4 h-4 mr-1" />
									{t('settings.connectedAccounts.disconnect')}
								{/if}
							</Button>
						{:else}
							<Button
								variant="primary"
								size="small"
								onclick={() => connect(provider)}
							>
								{t('settings.connectedAccounts.connect')} {provider.provider_type || t('settings.connectedAccounts.account')}
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
</div>
