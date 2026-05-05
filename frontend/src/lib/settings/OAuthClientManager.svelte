<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Plus, Edit2, Trash2, RefreshCw, Loader2, X } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import CopyButton from '../components/CopyButton.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import Input from '../components/Input.svelte';
	import FormField from '../components/FormField.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import SectionHeader from '../layout/SectionHeader.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { successToast, errorToast } from '../stores/toasts.svelte.js';
	import { confirm } from '../composables/useConfirm.js';

	// The list of scopes we let admins grant to OAuth clients. Mirrors the
	// `auth.AllValidScopes` Go-side allowlist, minus admin scopes (those are
	// rejected server-side anyway). Hardcoded for now — could be fetched from
	// /admin/scope-catalog later if the list grows.
	const SCOPE_OPTIONS = [
		{ value: 'items:read', label: 'Read items and comments' },
		{ value: 'items:write', label: 'Create and update items' },
		{ value: 'items:delete', label: 'Delete items' },
		{ value: 'workspaces:read', label: 'Read workspaces and configuration' },
		{ value: 'workspaces:write', label: 'Modify workspaces and configuration' },
		{ value: 'users:read', label: 'Read user directory' },
	];

	let clients = $state([]);
	let loading = $state(true);
	let error = $state('');
	let showFormModal = $state(false);
	let editingClient = $state(null);
	let saving = $state(false);

	// Plaintext-secret modal state. Populated after CreateClient or
	// RotateSecret returns; the secret is shown ONCE and then dropped.
	let secretModal = $state(/** @type {null | { client: any, secret: string }} */ (null));

	let formData = $state({
		slug: '',
		display_name: '',
		client_type: 'confidential',
		redirect_uris_text: '',
		allowed_scopes: /** @type {string[]} */ ([]),
		enabled: true,
	});

	onMount(() => {
		loadClients();
	});

	async function loadClients() {
		loading = true;
		error = '';
		try {
			clients = (await api.oauthClients.getAll()) || [];
		} catch (err) {
			console.error('Failed to load OAuth clients:', err);
			error = 'Failed to load OAuth clients';
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingClient = null;
		formData = {
			slug: '',
			display_name: '',
			client_type: 'confidential',
			redirect_uris_text: '',
			allowed_scopes: [],
			enabled: true,
		};
		showFormModal = true;
	}

	function openEdit(client) {
		editingClient = client;
		formData = {
			slug: client.slug, // immutable — shown disabled
			display_name: client.display_name,
			client_type: client.client_type, // immutable — shown disabled
			redirect_uris_text: (client.redirect_uris || []).join('\n'),
			allowed_scopes: [...(client.allowed_scopes || [])],
			enabled: client.enabled,
		};
		showFormModal = true;
	}

	function parseRedirectURIs(text) {
		return text
			.split('\n')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	function toggleScope(scope) {
		if (formData.allowed_scopes.includes(scope)) {
			formData.allowed_scopes = formData.allowed_scopes.filter((s) => s !== scope);
		} else {
			formData.allowed_scopes = [...formData.allowed_scopes, scope];
		}
	}

	async function save() {
		saving = true;
		try {
			const redirect_uris = parseRedirectURIs(formData.redirect_uris_text);

			if (editingClient) {
				// Update — only mutable fields. slug, client_id, client_type
				// are immutable post-creation.
				await api.oauthClients.update(editingClient.id, {
					display_name: formData.display_name,
					redirect_uris,
					allowed_scopes: formData.allowed_scopes,
					enabled: formData.enabled,
				});
				successToast('OAuth client updated');
				showFormModal = false;
				await loadClients();
			} else {
				// Create — server returns the plaintext client_secret exactly
				// once. Stash it in secretModal so the admin can copy it.
				const created = await api.oauthClients.create({
					slug: formData.slug,
					display_name: formData.display_name,
					client_type: formData.client_type,
					redirect_uris,
					allowed_scopes: formData.allowed_scopes,
					enabled: formData.enabled,
				});
				showFormModal = false;
				await loadClients();
				if (created?.client_secret) {
					secretModal = { client: created, secret: created.client_secret };
				} else {
					// Public client (no secret). Just confirm.
					successToast('Public OAuth client created');
				}
			}
		} catch (err) {
			console.error('Failed to save OAuth client:', err);
			errorToast(err.message || 'Failed to save OAuth client');
		} finally {
			saving = false;
		}
	}

	async function rotateSecret(client) {
		const ok = await confirm(
			`Rotate secret for "${client.display_name}"?`,
			'The previous secret stops working immediately. Existing access and refresh tokens keep working until they expire — only future /token exchanges that present the old secret start failing.'
		);
		if (!ok) return;

		try {
			const result = await api.oauthClients.rotateSecret(client.id);
			if (result?.client_secret) {
				secretModal = { client: result, secret: result.client_secret };
			}
			await loadClients();
		} catch (err) {
			console.error('Failed to rotate secret:', err);
			errorToast(err.message || 'Failed to rotate secret');
		}
	}

	async function deleteClient(client) {
		const ok = await confirm(
			`Delete "${client.display_name}"?`,
			'All access tokens and refresh tokens issued to this client will be revoked immediately. Users connected via this app will need to reconnect.'
		);
		if (!ok) return;

		try {
			await api.oauthClients.delete(client.id);
			successToast('OAuth client deleted');
			await loadClients();
		} catch (err) {
			console.error('Failed to delete OAuth client:', err);
			errorToast(err.message || 'Failed to delete OAuth client');
		}
	}

	const callbackHelp = $derived(
		'After this client redeems an authorization code at /api/oauth/token, every issued access token will be a Windshift API token (crw_…) bound to a per-user agent. Tokens inherit the requesting user\'s permissions, intersected with the scopes selected here.'
	);
</script>

<div>
	<SectionHeader
		title="OAuth Clients"
		subtitle="Register third-party apps that can authorize Windshift users via OAuth 2.0 (authorization code + PKCE). Each registered client can drive /api/oauth/authorize and /api/oauth/token to mint per-user API tokens."
		class="mb-6"
	>
		{#snippet actions()}
			<Button
				variant="primary"
				size="small"
				icon={Plus}
				onclick={openCreate}
				keyboardHint="A"
				hotkeyConfig={{ key: toHotkeyString('oauthClients', 'addClient'), guard: () => !showFormModal && !secretModal }}
			>
				Register Client
			</Button>
		{/snippet}
	</SectionHeader>

	{#if error}
		<div class="mb-4">
			<AlertBox message={error} />
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
		</div>
	{:else if clients.length === 0}
		<EmptyState
			title="No OAuth clients registered"
			description="Click 'Register Client' to add a third-party app."
		/>
	{:else}
		<div class="space-y-3">
			{#each clients as client}
				<div
					class="border rounded-lg p-4 flex items-center gap-4"
					style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
				>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2 flex-wrap">
							<h3 class="text-sm font-medium" style="color: var(--ds-text);">
								{client.display_name}
							</h3>
							<Lozenge appearance={client.enabled ? 'success' : 'default'}>
								{client.enabled ? 'Enabled' : 'Disabled'}
							</Lozenge>
							<Lozenge appearance="info">{client.client_type}</Lozenge>
							{#if (client.allowed_scopes || []).length > 0}
								<Lozenge appearance="default">
									{client.allowed_scopes.length} scope{client.allowed_scopes.length === 1 ? '' : 's'}
								</Lozenge>
							{/if}
						</div>
						<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
							<code>{client.client_id}</code>
							{#if client.client_type === 'confidential'}
								&middot; secret {client.has_secret ? 'set' : 'missing'}
							{/if}
							&middot; {(client.redirect_uris || []).length} redirect URI{(client.redirect_uris || []).length === 1 ? '' : 's'}
						</p>
					</div>
					<div class="flex items-center gap-1">
						{#if client.client_type === 'confidential'}
							<Button
								variant="ghost"
								size="small"
								title="Rotate client secret"
								onclick={() => rotateSecret(client)}
							>
								<RefreshCw class="w-4 h-4" />
							</Button>
						{/if}
						<Button variant="ghost" size="small" onclick={() => openEdit(client)}>
							<Edit2 class="w-4 h-4" />
						</Button>
						<Button variant="ghost" size="small" onclick={() => deleteClient(client)}>
							<Trash2 class="w-4 h-4" style="color: var(--ds-text-danger);" />
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create / Edit form -->
<Modal bind:isOpen={showFormModal}>
	<ModalHeader
		title={editingClient ? 'Edit OAuth Client' : 'Register OAuth Client'}
		onclose={() => (showFormModal = false)}
	/>

	<form
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
		class="p-4 space-y-4"
	>
		<FormField label="Display name" required>
			<Input
				bind:value={formData.display_name}
				placeholder="e.g. Omni — connector for Windshift"
			/>
		</FormField>

		<FormField label="Slug" required>
			<Input
				bind:value={formData.slug}
				placeholder="omni"
				disabled={!!editingClient}
			/>
		</FormField>

		<FormField label="Client type" required>
			<select
				bind:value={formData.client_type}
				disabled={!!editingClient}
				class="w-full h-9 px-3 text-sm border rounded-md disabled:opacity-60"
				style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
			>
				<option value="confidential">Confidential (server-to-server, has secret)</option>
				<option value="public">Public (PKCE only, no secret)</option>
			</select>
			<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
				Confidential clients store a client_secret server-side. Public clients (browser SPAs, native apps) must use PKCE on every /token exchange.
			</p>
		</FormField>

		<FormField label="Redirect URIs" required>
			<textarea
				bind:value={formData.redirect_uris_text}
				rows="3"
				class="w-full px-3 py-2 text-sm border rounded-md font-mono"
				style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
				placeholder={'https://app.example.com/oauth/callback\nhttps://staging.example.com/oauth/callback'}
			></textarea>
			<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
				One URI per line. The redirect_uri parameter on /authorize must exactly match one of these.
			</p>
		</FormField>

		<FormField label="Allowed scopes" required>
			<div class="space-y-2">
				{#each SCOPE_OPTIONS as scope}
					<label class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
						<input
							type="checkbox"
							checked={formData.allowed_scopes.includes(scope.value)}
							onchange={() => toggleScope(scope.value)}
						/>
						<code class="text-xs">{scope.value}</code>
						<span style="color: var(--ds-text-subtle);">— {scope.label}</span>
					</label>
				{/each}
			</div>
			<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
				{callbackHelp}
			</p>
		</FormField>

		<div class="flex items-center gap-2">
			<input type="checkbox" id="oauth-client-enabled" bind:checked={formData.enabled} />
			<label for="oauth-client-enabled" class="text-sm" style="color: var(--ds-text);">
				Enabled
			</label>
		</div>

		<div class="flex justify-end gap-2 pt-2">
			<Button variant="ghost" onclick={() => (showFormModal = false)}>Cancel</Button>
			<Button
				variant="primary"
				type="submit"
				disabled={saving ||
					!formData.slug ||
					!formData.display_name ||
					formData.allowed_scopes.length === 0 ||
					parseRedirectURIs(formData.redirect_uris_text).length === 0}
			>
				{#if saving}
					<Loader2 class="w-4 h-4 animate-spin mr-1" />
				{/if}
				{editingClient ? 'Update' : 'Register'}
			</Button>
		</div>
	</form>
</Modal>

<!-- Secret reveal — shown once after create or rotate -->
{#if secretModal}
	<Modal isOpen={true}>
		<ModalHeader
			title="Copy this secret now"
			onclose={() => (secretModal = null)}
		/>

		<div class="p-4 space-y-4">
			<AlertBox
				appearance="warning"
				message="The client secret is shown exactly once. Windshift only stores its bcrypt hash — once you close this dialog there is no way to recover it. If you lose it, use 'Rotate secret' to generate a new one."
			/>

			<div>
				<div class="text-xs mb-1" style="color: var(--ds-text-subtle);">Client ID</div>
				<div class="flex items-center gap-2">
					<code
						class="flex-1 text-xs px-3 py-2 rounded border break-all"
						style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
					>
						{secretModal.client.client_id}
					</code>
					<CopyButton getText={() => secretModal.client.client_id} title="Client ID" />
				</div>
			</div>

			<div>
				<div class="text-xs mb-1" style="color: var(--ds-text-subtle);">Client Secret</div>
				<div class="flex items-center gap-2">
					<code
						class="flex-1 text-xs px-3 py-2 rounded border break-all"
						style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
					>
						{secretModal.secret}
					</code>
					<CopyButton getText={() => secretModal.secret} title="Client Secret" />
				</div>
			</div>

			<div class="flex justify-end pt-2">
				<Button variant="primary" onclick={() => (secretModal = null)}>
					<X class="w-4 h-4 mr-1" /> Done
				</Button>
			</div>
		</div>
	</Modal>
{/if}
