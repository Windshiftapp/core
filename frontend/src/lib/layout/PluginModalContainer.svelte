<script>
	import { pluginModalRequests } from '../services/PluginBridge.js';
	import Modal from '../dialogs/Modal.svelte';
	import Button from '../components/Button.svelte';
	import Label from '../components/Label.svelte';
	import { t } from '../stores/i18n.svelte.js';

	let activeModals = $derived($pluginModalRequests);
</script>

<!-- Render all active plugin modals -->
{#each activeModals as modal (modal.id)}
	{#if modal.isConfirm}
		<!-- Confirm Dialog -->
		<Modal
			isOpen={true}
			onclose={() => modal.onClose('cancel')}
			maxWidth="max-w-md"
		>
			<div class="p-6">
				<h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
					{modal.title}
				</h3>
				<p class="mb-6" style="color: var(--ds-text);">
					{modal.message}
				</p>
				<div class="flex justify-end gap-2">
					<Button
						variant="secondary"
						onclick={() => modal.onClose('cancel')}
					>
						{modal.cancelText}
					</Button>
					<Button
						variant={modal.variant === 'danger' ? 'danger' : 'primary'}
						onclick={() => modal.onClose('confirm')}
					>
						{modal.confirmText}
					</Button>
				</div>
			</div>
		</Modal>
	{:else}
		<!-- Generic Modal -->
		<Modal
			isOpen={true}
			onclose={() => modal.onClose(null)}
			maxWidth={modal.maxWidth}
		>
			<!-- Modal Header -->
			<div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
				<h3 class="text-lg font-semibold" style="color: var(--ds-text);">
					{modal.title}
				</h3>
			</div>

			<!-- Modal Content -->
			<div class="px-6 py-4">
				{#if modal.content}
					<!-- Render content as structured data -->
					{#if typeof modal.content === 'object'}
						<div class="space-y-4">
							{#each Object.entries(modal.content) as [key, value]}
								<div>
									<Label class="mb-1">{key.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())}</Label>
									<div class="text-sm" style="color: var(--ds-text);">
										{#if typeof value === 'object'}
											<pre class="text-xs overflow-auto bg-gray-50 p-2 rounded">{JSON.stringify(value, null, 2)}</pre>
										{:else}
											{value || t('common.noData')}
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{:else}
						<div style="color: var(--ds-text);">{modal.content}</div>
					{/if}
				{/if}
			</div>

			<!-- Modal Footer -->
			<div class="px-6 py-4 border-t flex justify-end" style="border-color: var(--ds-border);">
				<Button onclick={() => modal.onClose(null)}>{t('common.close')}</Button>
			</div>
		</Modal>
	{/if}
{/each}
