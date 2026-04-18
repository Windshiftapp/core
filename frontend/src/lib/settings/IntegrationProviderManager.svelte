<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Plus, Edit2, Trash2, Loader2, Copy, Check } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import Input from '../components/Input.svelte';
	import FormField from '../components/FormField.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import { t } from '../stores/i18n.svelte.js';
	import { successToast, errorToast } from '../stores/toasts.svelte.js';
	import { confirm } from '../composables/useConfirm.js';

	const providerTypes = [
		{ value: 'notion', label: 'Notion' },
	];

	let providers = $state([]);
	let loading = $state(true);
	let error = $state('');
	let showModal = $state(false);
	let editingProvider = $state(null);
	let saving = $state(false);
	let copiedCallback = $state(false);

	let formData = $state({
		slug: '',
		name: '',
		provider_type: 'notion',
		enabled: true,
		oauth_client_id: '',
		oauth_client_secret: '',
		provider_config: '',
	});

	const callbackUrl = $derived(() => {
		const base = window.location.origin;
		return `${base}/api/integrations/oauth/${formData.slug || '{slug}'}/callback`;
	});

	onMount(() => {
		loadProviders();
	});

	async function loadProviders() {
		loading = true;
		error = '';
		try {
			providers = await api.integrationProviders.getAll() || [];
		} catch (err) {
			console.error('Failed to load providers:', err);
			error = 'Failed to load integration providers';
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingProvider = null;
		formData = {
			slug: '',
			name: '',
			provider_type: 'notion',
			enabled: true,
			oauth_client_id: '',
			oauth_client_secret: '',
			provider_config: '',
		};
		showModal = true;
	}

	function openEdit(provider) {
		editingProvider = provider;
		formData = {
			slug: provider.slug,
			name: provider.name,
			provider_type: provider.provider_type,
			enabled: provider.enabled,
			oauth_client_id: provider.oauth_client_id || '',
			oauth_client_secret: '',
			provider_config: provider.provider_config || '',
		};
		showModal = true;
	}

	async function save() {
		saving = true;
		try {
			const data = { ...formData };
			if (!data.oauth_client_secret) {
				delete data.oauth_client_secret;
			}
			if (!data.provider_config) {
				delete data.provider_config;
			}

			if (editingProvider) {
				await api.integrationProviders.update(editingProvider.id, data);
				successToast('Provider updated');
			} else {
				await api.integrationProviders.create(data);
				successToast('Provider created');
			}

			showModal = false;
			await loadProviders();
		} catch (err) {
			console.error('Failed to save provider:', err);
			errorToast(err.message || 'Failed to save provider');
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(provider) {
		const confirmed = await confirm(
			`Delete "${provider.name}"?`,
			'This will remove the integration provider and disconnect all users.'
		);
		if (!confirmed) return;

		try {
			await api.integrationProviders.delete(provider.id);
			successToast('Provider deleted');
			await loadProviders();
		} catch (err) {
			console.error('Failed to delete provider:', err);
			errorToast('Failed to delete provider');
		}
	}

	function copyCallbackUrl() {
		const url = callbackUrl();
		navigator.clipboard.writeText(url);
		copiedCallback = true;
		setTimeout(() => copiedCallback = false, 2000);
	}

	function getProviderLabel(type) {
		return providerTypes.find(pt => pt.value === type)?.label || type;
	}
</script>

<div>
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('integrations.providerManager')}</h2>
			<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{t('integrations.providerManagerDesc')}</p>
		</div>
		<Button variant="primary" size="small" onclick={openCreate}>
			<Plus class="w-4 h-4 mr-1" />
			{t('integrations.addProvider')}
		</Button>
	</div>

	{#if error}
		<div class="mb-4">
			<AlertBox message={error} />
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="w-6 h-6 animate-spin" style="color: var(--ds-text-subtle);" />
		</div>
	{:else if providers.length === 0}
		<EmptyState title={t('integrations.noProviders')} />
	{:else}
		<div class="space-y-3">
			{#each providers as provider}
				<div
					class="border rounded-lg p-4 flex items-center gap-4"
					style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
				>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<h3 class="text-sm font-medium" style="color: var(--ds-text);">{provider.name}</h3>
							<Lozenge appearance={provider.enabled ? 'success' : 'default'}>
								{provider.enabled ? 'Enabled' : 'Disabled'}
							</Lozenge>
							<Lozenge appearance="info">{getProviderLabel(provider.provider_type)}</Lozenge>
						</div>
						<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
							{provider.slug}
							{#if provider.has_oauth_client_secret}
								&middot; OAuth configured
							{/if}
						</p>
					</div>
					<div class="flex items-center gap-1">
						<Button variant="ghost" size="small" onclick={() => openEdit(provider)}>
							<Edit2 class="w-4 h-4" />
						</Button>
						<Button variant="ghost" size="small" onclick={() => deleteProvider(provider)}>
							<Trash2 class="w-4 h-4" style="color: var(--ds-text-danger);" />
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

	<Modal bind:isOpen={showModal}>
		<ModalHeader title={editingProvider ? t('integrations.editProvider') : t('integrations.addProvider')} onclose={() => showModal = false} />

		<form onsubmit={(e) => { e.preventDefault(); save(); }} class="p-4 space-y-4">
			<FormField label="Name" required>
				<Input bind:value={formData.name} placeholder="My Notion Integration" />
			</FormField>

			<FormField label="Slug" required>
				<Input bind:value={formData.slug} placeholder="notion-main" />
			</FormField>

			<FormField label={t('integrations.providerType')} required>
				<select
					bind:value={formData.provider_type}
					class="w-full h-9 px-3 text-sm border rounded-md"
					style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
				>
					{#each providerTypes as pt}
						<option value={pt.value}>{pt.label}</option>
					{/each}
				</select>
			</FormField>

			<FormField label={t('integrations.oauthClientId')}>
				<Input bind:value={formData.oauth_client_id} placeholder="OAuth Client ID" />
			</FormField>

			<FormField label={t('integrations.oauthClientSecret')}>
				<Input
					bind:value={formData.oauth_client_secret}
					type="password"
					placeholder={editingProvider?.has_oauth_client_secret ? '••••••••' : 'OAuth Client Secret'}
				/>
			</FormField>

			{#if formData.slug}
				<FormField label={t('integrations.callbackUrl')}>
					<div class="flex items-center gap-2">
						<code
							class="flex-1 text-xs px-3 py-2 rounded border overflow-x-auto"
							style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
						>
							{callbackUrl()}
						</code>
						<Button variant="ghost" size="small" onclick={copyCallbackUrl}>
							{#if copiedCallback}
								<Check class="w-4 h-4" style="color: var(--ds-text-success);" />
							{:else}
								<Copy class="w-4 h-4" />
							{/if}
						</Button>
					</div>
					<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('integrations.callbackUrlHint')}</p>
				</FormField>
			{/if}

			<div class="flex items-center gap-2">
				<input type="checkbox" id="int-enabled" bind:checked={formData.enabled} />
				<label for="int-enabled" class="text-sm" style="color: var(--ds-text);">Enabled</label>
			</div>

			<div class="flex justify-end gap-2 pt-2">
				<Button variant="ghost" onclick={() => showModal = false}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={saving || !formData.slug || !formData.name}>
					{#if saving}
						<Loader2 class="w-4 h-4 animate-spin mr-1" />
					{/if}
					{editingProvider ? 'Update' : 'Create'}
				</Button>
			</div>
		</form>
	</Modal>
