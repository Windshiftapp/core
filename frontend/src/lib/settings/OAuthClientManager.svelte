<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Plus, Edit2, Trash2, RefreshCw, Loader2, X } from '@lucide/svelte';
	import Button from '../components/Button.svelte';
	import CopyButton from '../components/CopyButton.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import Input from '../components/Input.svelte';
	import Checkbox from '../components/Checkbox.svelte';
	import NativeSelect from '../components/NativeSelect.svelte';
	import Textarea from '../components/Textarea.svelte';
	import FormField from '../components/FormField.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import DropdownMenu from '../layout/DropdownMenu.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import SectionHeader from '../layout/SectionHeader.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { successToast, errorToast } from '../stores/toasts.svelte.js';
	import { confirm } from '../composables/useConfirm.js';
	import { t } from '../stores/i18n.svelte.js';

	// The scopes an admin may grant an OAuth client, fetched from the server
	// catalog (auth.ScopeCatalog) minus admin scopes, which the OAuth surfaces
	// reject outright. Previously hand-maintained here, which is what let the
	// list fall behind the server allowlist.
	let scopeCatalog = $state([]);
	let scopeOptions = $derived(scopeCatalog.filter((s) => !s.admin));

	async function loadScopeCatalog() {
		try {
			scopeCatalog = (await api.getScopeCatalog()) || [];
		} catch (err) {
			console.warn('Failed to load scope catalog:', err);
			scopeCatalog = [];
		}
	}

	const DOCMOST_REQUIRED_SCOPES = ['items:read', 'workspaces:read', 'collections:read'];
	const DOCMOST_LOCAL_CALLBACK = 'http://localhost:3000/api/integrations/oauth/windshift/callback';

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
		loadScopeCatalog();
	});

	async function loadClients() {
		loading = true;
		error = '';
		try {
			clients = (await api.oauthClients.getAll()) || [];
		} catch (err) {
			console.error('Failed to load OAuth clients:', err);
			error = t('integrations.oauthClients.loadFailed');
		} finally {
			loading = false;
		}
	}

	function blankFormData() {
		return {
			slug: '',
			display_name: '',
			client_type: 'confidential',
			redirect_uris_text: '',
			allowed_scopes: [],
			enabled: true,
		};
	}

	function docmostTemplateFormData() {
		return {
			slug: 'docmost',
			display_name: 'Docmost',
			client_type: 'confidential',
			redirect_uris_text: DOCMOST_LOCAL_CALLBACK,
			allowed_scopes: DOCMOST_REQUIRED_SCOPES,
			enabled: true,
		};
	}

	function openCreate() {
		editingClient = null;
		formData = blankFormData();
		showFormModal = true;
	}

	function openCreateFromTemplate(template) {
		editingClient = null;
		if (template === 'docmost') {
			formData = docmostTemplateFormData();
		} else {
			formData = blankFormData();
		}
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
				successToast(t('integrations.oauthClients.updated'));
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
					successToast(t('integrations.oauthClients.publicCreated'));
				}
			}
		} catch (err) {
			console.error('Failed to save OAuth client:', err);
			errorToast(err.message || t('integrations.oauthClients.saveFailed'));
		} finally {
			saving = false;
		}
	}

	async function rotateSecret(client) {
		const ok = await confirm(
			t('integrations.oauthClients.rotateTitle', { name: client.display_name }),
			t('integrations.oauthClients.rotateMessage')
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
			errorToast(err.message || t('integrations.oauthClients.rotateFailed'));
		}
	}

	async function deleteClient(client) {
		const ok = await confirm(
			t('integrations.oauthClients.deleteTitle', { name: client.display_name }),
			t('integrations.oauthClients.deleteMessage')
		);
		if (!ok) return;

		try {
			await api.oauthClients.delete(client.id);
			successToast(t('integrations.oauthClients.deleted'));
			await loadClients();
		} catch (err) {
			console.error('Failed to delete OAuth client:', err);
			errorToast(err.message || t('integrations.oauthClients.deleteFailed'));
		}
	}

	const callbackHelp = $derived(t('integrations.oauthClients.scopeHelp'));

	function displayClientId(clientId) {
		if (!clientId || clientId.length <= 16) return clientId || '';
		return `${clientId.slice(0, 9)}…${clientId.slice(-6)}`;
	}
</script>

<div>
	<SectionHeader
		title={t('integrations.oauthClients.title')}
		subtitle={t('integrations.oauthClients.subtitle')}
		class="mb-6"
	>
		{#snippet actions()}
			<div class="flex items-center gap-2">
				<DropdownMenu
					triggerText={t('integrations.oauthClients.template')}
					placement="bottom-end"
					maxWidth="max-w-sm"
					triggerClass="px-3.5 py-1.5 border"
					triggerStyle="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
					items={[
						{
							id: 'docmost',
							title: 'Docmost',
							subtitle: `${DOCMOST_REQUIRED_SCOPES.join(' ')} · ${t('integrations.oauthClients.oauthCallback')}`,
							onClick: () => openCreateFromTemplate('docmost'),
						},
					]}
				/>
				<Button
					variant="primary"
					size="small"
					icon={Plus}
					onclick={openCreate}
					keyboardHint="A"
					hotkeyConfig={{ key: toHotkeyString('oauthClients', 'addClient'), guard: () => !showFormModal && !secretModal }}
				>
					{t('integrations.oauthClients.registerClient')}
				</Button>
			</div>
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
			title={t('integrations.oauthClients.empty')}
			description={t('integrations.oauthClients.emptyDescription')}
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
								{client.enabled ? t('common.enabled') : t('common.disabled')}
							</Lozenge>
							<Lozenge appearance="info">{client.client_type}</Lozenge>
							{#if (client.allowed_scopes || []).length > 0}
								<Lozenge appearance="default">
									{t('integrations.oauthClients.scopeCount', { count: client.allowed_scopes.length })}
								</Lozenge>
							{/if}
						</div>
						<p class="text-xs mt-1 flex items-center gap-1 flex-wrap" style="color: var(--ds-text-subtle);">
							<code title={client.client_id}>{displayClientId(client.client_id)}</code>
							<CopyButton getText={() => client.client_id} title={t('integrations.oauthClients.clientId')} />
							{#if client.client_type === 'confidential'}
								<span>&middot; {client.has_secret ? t('integrations.oauthClients.secretSet') : t('integrations.oauthClients.secretMissing')}</span>
							{/if}
							&middot; {t('integrations.oauthClients.redirectCount', { count: (client.redirect_uris || []).length })}
						</p>
					</div>
					<div class="flex items-center gap-1">
						{#if client.client_type === 'confidential'}
							<Button
								variant="ghost"
								size="small"
								title={t('integrations.oauthClients.rotateSecret')}
								onclick={() => rotateSecret(client)}
							>
								<RefreshCw class="w-4 h-4" />
							</Button>
						{/if}
						<Button variant="ghost" size="small" onclick={() => openEdit(client)}>
							<Edit2 class="w-4 h-4" />
						</Button>
						<Button variant="danger-ghost" size="small" icon={Trash2} title={t('integrations.oauthClients.deleteClient')} onclick={() => deleteClient(client)}></Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create / Edit form -->
<Modal bind:isOpen={showFormModal}>
	<ModalHeader
		title={editingClient ? t('integrations.oauthClients.editTitle') : t('integrations.oauthClients.registerTitle')}
		onclose={() => (showFormModal = false)}
	/>

	<form
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
		class="p-4 space-y-4"
	>
		<FormField label={t('integrations.oauthClients.displayName')} required>
			<Input
				bind:value={formData.display_name}
				placeholder={t('integrations.oauthClients.displayNamePlaceholder')}
			/>
		</FormField>

		<FormField label={t('integrations.oauthClients.slug')} required>
			<Input
				bind:value={formData.slug}
				placeholder="omni"
				disabled={!!editingClient}
			/>
		</FormField>

		<FormField label={t('integrations.oauthClients.clientType')} required>
			<NativeSelect
				bind:value={formData.client_type}
				disabled={!!editingClient}
				options={[
					{ value: 'confidential', label: t('integrations.oauthClients.confidential') },
					{ value: 'public', label: t('integrations.oauthClients.public') },
				]}
				size="small"
			/>
			<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
				{t('integrations.oauthClients.clientTypeHelp')}
			</p>
		</FormField>

		<FormField label={t('integrations.oauthClients.redirectUris')} required>
			<Textarea
				bind:value={formData.redirect_uris_text}
				rows={3}
				class="font-mono"
				size="small"
				placeholder={`https://docmost.example.com/api/integrations/oauth/windshift/callback\n${DOCMOST_LOCAL_CALLBACK}`}
			/>
			<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
				{t('integrations.oauthClients.redirectHelp')}
			</p>
		</FormField>

		<FormField label={t('integrations.oauthClients.allowedScopes')} required>
			<div class="space-y-2">
				{#each scopeOptions as scope (scope.scope)}
					<Checkbox
						checked={formData.allowed_scopes.includes(scope.scope)}
						onchange={() => toggleScope(scope.scope)}
						dataTestid={`oauth-client-scope-${scope.scope}`}
						label={`${scope.scope} — ${scope.label}`}
						size="small"
					/>
				{/each}
			</div>
			<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
				{callbackHelp}
			</p>
		</FormField>

		<Checkbox id="oauth-client-enabled" bind:checked={formData.enabled} label={t('common.enabled')} size="small" />

		<div class="flex justify-end gap-2 pt-2">
			<Button variant="ghost" onclick={() => (showFormModal = false)}>{t('common.cancel')}</Button>
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
				{editingClient ? t('common.update') : t('integrations.oauthClients.register')}
			</Button>
		</div>
	</form>
</Modal>

<!-- Secret reveal — shown once after create or rotate -->
{#if secretModal}
	<Modal isOpen={true}>
		<ModalHeader
			title={t('integrations.oauthClients.copySecretNow')}
			onclose={() => (secretModal = null)}
		/>

		<div class="p-4 space-y-4">
			<AlertBox
				appearance="warning"
				message={t('integrations.oauthClients.secretWarning')}
			/>

			<div>
				<div class="text-xs mb-1" style="color: var(--ds-text-subtle);">{t('integrations.oauthClients.clientId')}</div>
				<div class="flex items-center gap-2">
					<code
						class="flex-1 text-xs px-3 py-2 rounded border break-all"
						style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
					>
						{secretModal.client.client_id}
					</code>
					<CopyButton getText={() => secretModal.client.client_id} title={t('integrations.oauthClients.clientId')} />
				</div>
			</div>

			<div>
				<div class="text-xs mb-1" style="color: var(--ds-text-subtle);">{t('integrations.oauthClients.clientSecret')}</div>
				<div class="flex items-center gap-2">
					<code
						class="flex-1 text-xs px-3 py-2 rounded border break-all"
						style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
					>
						{secretModal.secret}
					</code>
					<CopyButton getText={() => secretModal.secret} title={t('integrations.oauthClients.clientSecret')} />
				</div>
			</div>

			<div class="flex justify-end pt-2">
				<Button variant="primary" onclick={() => (secretModal = null)}>
					<X class="w-4 h-4 mr-1" /> {t('common.done')}
				</Button>
			</div>
		</div>
	</Modal>
{/if}
