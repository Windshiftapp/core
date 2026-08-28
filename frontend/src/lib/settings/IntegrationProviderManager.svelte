<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Plus, Edit2, Trash2, Loader2 } from '@lucide/svelte';
	import Button from '../components/Button.svelte';
	import CopyButton from '../components/CopyButton.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import Input from '../components/Input.svelte';
	import Checkbox from '../components/Checkbox.svelte';
	import NativeSelect from '../components/NativeSelect.svelte';
	import FormField from '../components/FormField.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import SectionHeader from '../layout/SectionHeader.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { t } from '../stores/i18n.svelte.js';
	import { successToast, errorToast } from '../stores/toasts.svelte.js';
	import { confirm } from '../composables/useConfirm.js';
	import { publicBaseURL } from '../runtime/contextPath.js';

	const providerTypes = [
		{ value: 'notion', label: 'Notion' },
		{ value: 'todoist', label: 'Todoist' },
	];

	let providers = $state([]);
	let loading = $state(true);
	let error = $state('');
	let showModal = $state(false);
	let editingProvider = $state(null);
	let saving = $state(false);

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
		return `${publicBaseURL()}/api/integrations/oauth/${formData.slug || '{slug}'}/callback`;
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
			error = t('integrations.providers.loadFailed');
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
				successToast(t('integrations.providers.updated'));
			} else {
				await api.integrationProviders.create(data);
				successToast(t('integrations.providers.created'));
			}

			showModal = false;
			await loadProviders();
		} catch (err) {
			console.error('Failed to save provider:', err);
			errorToast(err.message || t('integrations.providers.saveFailed'));
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(provider) {
		const confirmed = await confirm(
			t('integrations.providers.deleteTitle', { name: provider.name }),
			t('integrations.providers.deleteMessage')
		);
		if (!confirmed) return;

		try {
			await api.integrationProviders.delete(provider.id);
			successToast(t('integrations.providers.deleted'));
			await loadProviders();
		} catch (err) {
			console.error('Failed to delete provider:', err);
			errorToast(t('integrations.providers.deleteFailed'));
		}
	}

	function getProviderLabel(type) {
		return providerTypes.find(pt => pt.value === type)?.label || type;
	}
</script>

<div>
	<SectionHeader
		title={t('integrations.providerManager')}
		subtitle={t('integrations.providerManagerDesc')}
		class="mb-6"
	>
		{#snippet actions()}
			<Button
				variant="primary"
				size="small"
				icon={Plus}
				onclick={openCreate}
				keyboardHint="A"
				hotkeyConfig={{ key: toHotkeyString('integrationProviders', 'addProvider'), guard: () => !showModal }}
			>
				{t('integrations.addProvider')}
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
				{provider.enabled ? t('common.enabled') : t('common.disabled')}
							</Lozenge>
							<Lozenge appearance="info">{getProviderLabel(provider.provider_type)}</Lozenge>
						</div>
						<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
							{provider.slug}
							{#if provider.has_oauth_client_secret}
				&middot; {t('integrations.providers.oauthConfigured')}
							{/if}
						</p>
					</div>
					<div class="flex items-center gap-1">
						<Button variant="ghost" size="small" onclick={() => openEdit(provider)}>
							<Edit2 class="w-4 h-4" />
						</Button>
						<Button variant="danger-ghost" size="small" icon={Trash2} title={t('common.delete')} onclick={() => deleteProvider(provider)}></Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

	<Modal bind:isOpen={showModal}>
		<ModalHeader title={editingProvider ? t('integrations.editProvider') : t('integrations.addProvider')} onclose={() => showModal = false} />

		<form onsubmit={(e) => { e.preventDefault(); save(); }} class="p-4 space-y-4">
			<FormField label={t('common.name')} required>
				<Input bind:value={formData.name} placeholder={t('integrations.providers.namePlaceholder')} />
			</FormField>

			<FormField label={t('integrations.providers.slug')} required>
				<Input bind:value={formData.slug} placeholder="notion-main" />
			</FormField>

			<FormField label={t('integrations.providerType')} required>
				<NativeSelect
					bind:value={formData.provider_type}
					options={providerTypes}
					size="small"
				/>
			</FormField>

			<FormField label={t('integrations.oauthClientId')}>
				<Input bind:value={formData.oauth_client_id} placeholder={t('integrations.oauthClientId')} />
			</FormField>

			<FormField label={t('integrations.oauthClientSecret')}>
				<Input
					bind:value={formData.oauth_client_secret}
					type="password"
					placeholder={editingProvider?.has_oauth_client_secret ? '••••••••' : t('integrations.oauthClientSecret')}
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
						<CopyButton getText={callbackUrl} title={t('integrations.callbackUrl')} />
					</div>
					<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('integrations.callbackUrlHint')}</p>
				</FormField>
			{/if}

			<Checkbox id="int-enabled" bind:checked={formData.enabled} label={t('common.enabled')} size="small" />

			<div class="flex justify-end gap-2 pt-2">
				<Button variant="ghost" onclick={() => showModal = false}>{t('common.cancel')}</Button>
				<Button variant="primary" type="submit" disabled={saving || !formData.slug || !formData.name}>
					{#if saving}
						<Loader2 class="w-4 h-4 animate-spin mr-1" />
					{/if}
					{editingProvider ? t('common.update') : t('common.create')}
				</Button>
			</div>
		</form>
	</Modal>
