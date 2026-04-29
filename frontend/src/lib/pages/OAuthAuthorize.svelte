<script>
	// /oauth/authorize?... — the consent screen for the generic OAuth 2.0
	// server. A third-party app redirected the browser here; on Allow we POST
	// to /api/oauth/authorize/approve, get back a redirect_to URL with a
	// fresh `code`, and bounce the browser to it. On Deny we POST /deny and
	// bounce with `error=access_denied`.
	//
	// Pairs with internal/handlers/oauth.go.

	import { onMount } from 'svelte';
	import { Lock, AlertTriangle } from 'lucide-svelte';
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { currentRoute } from '../router.js';
	import ConsentCard from '../components/ConsentCard.svelte';
	import Spinner from '../components/Spinner.svelte';

	const SCOPE_DESCRIPTIONS = {
		'items:read': 'Read work items and comments',
		'items:write': 'Create and update work items',
		'items:delete': 'Delete work items',
		'workspaces:read': 'Read workspaces and configuration',
		'workspaces:write': 'Modify workspaces and configuration',
		'workspaces:delete': 'Delete workspaces',
		'users:read': 'Read user directory',
	};

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
	}} */ (null));
	let infoLoading = $state(true);
	let infoError = $state('');
	let working = $state(false);
	let actionError = $state('');

	onMount(async () => {
		try {
			info = await api.oauth.authorizeInfo({
				client_id: params.client_id || '',
				redirect_uri: params.redirect_uri || '',
				response_type: params.response_type || 'code',
				scope: params.scope || '',
				state: params.state || '',
				code_challenge: params.code_challenge || '',
				code_challenge_method: params.code_challenge_method || '',
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
		scopeDescriptions={SCOPE_DESCRIPTIONS}
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
				Windshift will create a dedicated agent for this app and issue API tokens it can use to call Windshift on your behalf. You can revoke access any time from your profile's Agents tab.
			</p>
			<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
				Browser will redirect to: <code class="break-all">{info.redirect_uri}</code>
			</p>
		</div>
	</ConsentCard>
{/if}
