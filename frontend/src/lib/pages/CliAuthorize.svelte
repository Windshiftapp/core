<script>
	// /cli/authorize?... — the consent screen for the `ws init` CLI flow.
	// Shares the ConsentCard layout with OAuthAuthorize.svelte so both
	// "external thing wants to act on your behalf" experiences look the same.
	//
	// Pairs with internal/handlers/cli_auth.go.

	import { onMount } from 'svelte';
	import { Terminal, Shield, AlertTriangle } from '@lucide/svelte';
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { currentRoute } from '../router.js';
	import ConsentCard from '../components/ConsentCard.svelte';
	import Button from '../components/Button.svelte';
	import Spinner from '../components/Spinner.svelte';
	import { scopeCatalogStore } from '../stores/scopeCatalog.svelte.js';

	// Scope descriptions come from the server catalog (auth.ScopeCatalog) so the
	// consent screen can explain every scope a client might request. The previous
	// hand-written map covered 8 of them and silently fell back to showing the
	// raw scope string for the rest.
	let scopeDescriptions = $derived(
		Object.fromEntries(scopeCatalogStore.catalog.map((s) => [s.scope, s.description]))
	);

	let params = $derived($currentRoute.query || {});
	let callbackURL = $derived(params.callback || '');
	let routeState = $derived(params.state || '');
	let hostname = $derived(params.hostname || 'this machine');
	let agentName = $derived(params.agent_name || 'ws-cli');
	let scopes = $derived(
		params.scope
			? params.scope
					.split(',')
					.map((s) => s.trim())
					.filter(Boolean)
			: []
	);

	let caps = $state(null);
	let capsLoading = $state(true);
	let working = $state(false);
	let completed = $state(false);
	let error = $state('');

	onMount(async () => {
		scopeCatalogStore.load();
		try {
			caps = await api.cliAuth.capabilities();
		} catch (err) {
			console.error('Failed to load CLI capabilities', err);
			error = 'Unable to reach the server. Please try again.';
		} finally {
			capsLoading = false;
		}
	});

	function buildCallback(extra) {
		try {
			const url = new URL(callbackURL);
			for (const [k, v] of Object.entries(extra)) {
				url.searchParams.set(k, v);
			}
			return url.toString();
		} catch {
			return null;
		}
	}

	function paramsComplete() {
		return callbackURL && routeState && agentName;
	}

	async function approve() {
		if (working || completed) return;
		working = true;
		error = '';
		try {
			const resp = await api.cliAuth.approve({
				callback_url: callbackURL,
				state: routeState,
				hostname: params.hostname || '',
				agent_name: agentName,
				first_name: 'ws-cli',
				last_name: params.hostname || 'agent',
				scopes,
			});
			const redirect = buildCallback({
				code: resp.code,
				state: resp.state,
				result: 'ok',
			});
			if (!redirect) {
				error = 'Invalid callback URL supplied by the CLI.';
				return;
			}
			completed = true;
			window.location.replace(redirect);
		} catch (err) {
			console.error('CLI approve failed', err);
			error = err?.data?.error || err?.message || 'Approval failed.';
		} finally {
			working = false;
		}
	}

	async function deny() {
		if (working || completed) return;
		working = true;
		error = '';
		try {
			await api.cliAuth.deny({
				hostname: params.hostname || '',
				agent_name: agentName,
			});
		} catch (err) {
			console.warn('CLI deny audit failed (continuing)', err);
		}
		const redirect = buildCallback({ state: routeState, result: 'denied' });
		completed = true;
		if (redirect) {
			window.location.replace(redirect);
		} else {
			working = false;
		}
	}
</script>

{#if capsLoading}
	<div
		class="min-h-screen flex items-center justify-center"
		style="background-color: var(--ds-surface);"
	>
		<Spinner />
	</div>
{:else if !paramsComplete()}
	<div
		class="min-h-screen flex items-center justify-center px-4 py-10"
		style="background-color: var(--ds-surface);"
		data-testid="cli-parameters-error"
	>
		<div
			class="w-full max-w-lg rounded-lg border p-6 shadow-sm"
			style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
		>
			<div class="flex items-center gap-3 mb-4">
				<AlertTriangle size={22} style="color: var(--ds-text-subtle);" />
				<h1 class="text-lg font-semibold" style="color: var(--ds-text);">
					CLI parameters missing
				</h1>
			</div>
			<p class="text-sm" style="color: var(--ds-text);">
				This page is meant to be opened by the <code>ws</code> CLI. It cannot be used directly — some parameters are missing.
			</p>
		</div>
	</div>
{:else if !caps?.auto_onboarding_enabled}
	<div
		class="min-h-screen flex items-center justify-center px-4 py-10"
		style="background-color: var(--ds-surface);"
		data-testid="cli-disabled"
	>
		<div
			class="w-full max-w-lg rounded-lg border p-6 shadow-sm"
			style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
		>
			<div class="flex items-center gap-3 mb-4">
				<Shield size={22} style="color: var(--ds-text-subtle);" />
				<h1 class="text-lg font-semibold" style="color: var(--ds-text);">
					Automatic setup is disabled on this instance
				</h1>
			</div>
			<p class="text-sm mb-3" style="color: var(--ds-text-subtle);">
				{#if !caps?.agents_enabled}
					Your admin hasn't enabled user-managed agents, so the CLI can't mint its own token.
				{:else if !caps?.manual_tokens_enabled}
					API token creation is disabled on this instance.
				{:else}
					Automatic setup is not available.
				{/if}
			</p>
			{#if caps?.manual_tokens_enabled}
				<p class="text-sm" style="color: var(--ds-text-subtle);">You can still set up the CLI manually:</p>
				<ol class="text-sm list-decimal ml-5 mt-2 space-y-1" style="color: var(--ds-text);">
					<li>Cancel this flow (the CLI will fall back to manual).</li>
					<li>In the CLI, run <code>ws init --manual</code>.</li>
					<li>Create a personal API token in your profile and paste it when prompted.</li>
				</ol>
			{:else}
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					Please ask your administrator to enable user-managed agents or API token creation.
				</p>
			{/if}
			<div class="mt-4 flex justify-end">
				<Button variant="default" onclick={deny} disabled={working}>Close</Button>
			</div>
		</div>
	</div>
{:else}
	<ConsentCard
		icon={Terminal}
		title="Authorize Windshift CLI"
		{scopes}
		{scopeDescriptions}
		{error}
		onAllow={approve}
		onDeny={deny}
		loading={working}
		disabled={completed}
	>
		{#snippet subtitleSnippet()}
			The <code>ws</code> CLI running on <strong>{hostname}</strong> is asking to connect.
		{/snippet}
		<div
			class="rounded-md border p-4"
			style="border-color: var(--ds-border); background-color: var(--ds-surface);"
		>
			<div class="text-xs uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
				You are signing in as
			</div>
			<div class="text-sm font-medium" style="color: var(--ds-text);">
				{authStore.currentUser?.full_name || authStore.currentUser?.username || '—'}
			</div>
			<div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
				{authStore.currentUser?.email || ''}
			</div>
		</div>

		<div
			class="rounded-md border p-4"
			style="border-color: var(--ds-border); background-color: var(--ds-surface);"
		>
			<div class="text-xs uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
				Agent that will be created / reused
			</div>
			<div class="text-sm font-mono" style="color: var(--ds-text);">{agentName}</div>
			<div class="text-xs mt-2" style="color: var(--ds-text-subtle);">
				The CLI will act as this agent user on your behalf. You can revoke it any time from your profile's Agents tab.
			</div>
		</div>
	</ConsentCard>
{/if}
