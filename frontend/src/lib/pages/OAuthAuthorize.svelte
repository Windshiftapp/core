<script>
	// Generic OAuth consent: approve redirects with a fresh code; denial returns
	// error=access_denied.

	import { onMount } from 'svelte';
	import { Lock, AlertTriangle } from '@lucide/svelte';
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { currentRoute } from '../router.js';
	import ConsentCard from '../components/ConsentCard.svelte';
	import Spinner from '../components/Spinner.svelte';
	import { scopeCatalogStore } from '../stores/scopeCatalog.svelte.js';

	// Scope descriptions come from the server catalog (auth.ScopeCatalog) so the
	// consent screen can explain every scope a client might request. The previous
	// hand-written map covered 8 of them and silently fell back to showing the
	// raw scope string for the rest.
	let scopeDescriptions = $derived(
		Object.fromEntries(scopeCatalogStore.catalog.map((s) => [s.scope, s.description]))
	);

	// Parse the query string the third-party app sent the browser with.
	const params = $derived($currentRoute.query || {});

	let info = $state(/** @type {null | {
		client_id: string,
		client_display_name: string,
		redirect_uri: string,
		granted_scopes: string[],
		state: string,
		code_challenge?: string,
		code_challenge_method?: string,
		resource?: string,
	}} */ (null));
	let infoLoading = $state(true);
	let infoError = $state('');
	let working = $state(false);
	let actionError = $state('');

	onMount(async () => {
		scopeCatalogStore.load();
		try {
			info = await api.oauth.authorizeInfo({
				client_id: params.client_id || '',
				redirect_uri: params.redirect_uri || '',
				response_type: params.response_type || 'code',
				scope: params.scope || '',
				state: params.state || '',
				code_challenge: params.code_challenge || '',
				code_challenge_method: params.code_challenge_method || '',
				resource: params.resource || '',
			});
		} catch (err) {
			console.error('OAuth /info failed', err);
			infoError = err?.data?.error || err?.message || 'Invalid authorization request';
		} finally {
			infoLoading = false;
		}
	});

	function approveBody() {
		return {
			client_id: info.client_id,
			redirect_uri: info.redirect_uri,
			response_type: 'code',
			scope: info.granted_scopes.join(' '),
			state: info.state,
			code_challenge: info.code_challenge || '',
			code_challenge_method: info.code_challenge_method || '',
			resource: info.resource || '',
		};
	}

	async function approve() {
		if (working) return;
		working = true;
		actionError = '';
		try {
			const resp = await api.oauth.authorizeApprove(approveBody());
			if (resp?.redirect_to) {
				window.location.replace(resp.redirect_to);
				return;
			}
			actionError = 'Approval succeeded but the server did not return a redirect URL.';
		} catch (err) {
			console.error('OAuth approve failed', err);
			actionError = err?.data?.error_description || err?.data?.error || err?.message || 'Approval failed.';
		} finally {
			working = false;
		}
	}

	async function deny() {
		if (working) return;
		working = true;
		actionError = '';
		try {
			const resp = await api.oauth.authorizeDeny(approveBody());
			if (resp?.redirect_to) {
				window.location.replace(resp.redirect_to);
				return;
			}
		} catch (err) {
			console.warn('OAuth deny audit failed (continuing)', err);
		} finally {
			working = false;
		}
	}
</script>

{#if infoLoading}
	<div
		class="min-h-screen flex items-center justify-center"
		style="background-color: var(--ds-surface);"
	>
		<Spinner />
	</div>
{:else if infoError}
	<div
		class="min-h-screen flex items-center justify-center px-4 py-10"
		style="background-color: var(--ds-surface);"
		data-testid="oauth-authorize-error"
	>
		<div
			class="w-full max-w-lg rounded-lg border p-6 shadow-sm"
			style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
		>
			<div class="flex items-center gap-3 mb-4">
				<AlertTriangle size={22} style="color: var(--ds-text-subtle);" />
				<h1 class="text-lg font-semibold" style="color: var(--ds-text);">
					Cannot authorize this request
				</h1>
			</div>
			<p class="text-sm" style="color: var(--ds-text);">{infoError}</p>
			<p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
				This usually means the redirecting app's request is malformed (unknown client_id, mismatched redirect_uri, or invalid scope). Contact the app's administrator.
			</p>
		</div>
	</div>
{:else if info}
	<ConsentCard
		icon={Lock}
		title="Authorize {info.client_display_name}"
		scopes={info.granted_scopes}
		{scopeDescriptions}
		error={actionError}
		onAllow={approve}
		onDeny={deny}
		loading={working}
	>
		{#snippet subtitleSnippet()}
			<strong>{info.client_display_name}</strong> wants to act on your behalf in Windshift.
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
				After you approve
			</div>
			<p class="text-sm" style="color: var(--ds-text);">
				Windshift will create a dedicated agent for this app and issue API tokens it can use to call Windshift on your behalf only. Other users must authorize the app with their own Windshift accounts. You can revoke your access any time from your profile's Agents tab.
			</p>
			<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
				Browser will redirect to: <code class="break-all">{info.redirect_uri}</code>
			</p>
		</div>
	</ConsentCard>
{/if}
