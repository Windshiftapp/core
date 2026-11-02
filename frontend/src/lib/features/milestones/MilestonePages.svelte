<script>
	import { onMount } from 'svelte';
	import { FileText, Plus, Trash2 } from '@lucide/svelte';
	import { milestones as milestonesApi } from '../../api/milestones.js';
	import AlertBox from '../../components/AlertBox.svelte';
	import LinkComponent from '../../components/Link.svelte';
	import DialogFooter from '../../dialogs/DialogFooter.svelte';
	import Modal from '../../dialogs/Modal.svelte';
	import ModalHeader from '../../dialogs/ModalHeader.svelte';
	import PagePicker from '../../pickers/PagePicker.svelte';
	import { errorToast } from '../../stores/toasts.svelte.js';
	import { t } from '../../stores/i18n.svelte.js';

	// Pages linked to a workspace milestone, shown as a list in the
	// milestone header card. Global milestones have no workspace and
	// therefore no Pages section. Anyone who can view the milestone sees the
	// linked pages they may view; users with edit rights on the milestone
	// (canEdit) link pages from its workspace through "+ Add", which opens a
	// link dialog like the item detail's Pages "+ Add", and unlink
	// them per row.
	let { milestoneId, workspaceId, canEdit = false } = $props();

	let links = $state([]);
	let loading = $state(true);
	let error = $state('');
	let addButton = $state(null);

	// Link dialog state.
	let dialogOpen = $state(false);
	let selectedPageId = $state(null);
	let selectedPage = $state(null);
	let linking = $state(false);

	const linkedPageIds = $derived(new Set(links.map((link) => link.page_id)));
	const selectedAlreadyLinked = $derived(!!selectedPage && linkedPageIds.has(selectedPage.id));
	const canSubmit = $derived(!!selectedPage && !selectedAlreadyLinked && !linking);
	// Editors always see the heading and "+ Add"; viewers only once pages (or a
	// load error) are there to show.
	const visible = $derived(canEdit || (!loading && (links.length > 0 || !!error)));

	onMount(() => {
		loadLinks();
	});

	async function loadLinks() {
		loading = true;
		error = '';
		try {
			links = (await milestonesApi.getPageLinks(milestoneId)) || [];
		} catch (err) {
			console.error('Failed to load milestone page links:', err);
			error = t('dialogs.alerts.failedToLoad', { error: t('items.linkedPages') });
			links = [];
		} finally {
			loading = false;
		}
	}

	function openDialog() {
		selectedPageId = null;
		selectedPage = null;
		dialogOpen = true;
	}

	// Cancel, Escape, a backdrop click and a successful link all end here.
	// Modal does not restore focus itself, so return it to "+ Add".
	function closeDialog() {
		if (linking) return;
		dialogOpen = false;
		selectedPageId = null;
		selectedPage = null;
		addButton?.focus();
	}

	function handleSelectPage(page) {
		selectedPage = page || null;
	}

	// PagePicker marks Escape as handled (preventDefault), which stops Modal
	// from closing on it. Listen in the capture phase so Escape from the
	// search box still closes the dialog, except while the picker's dropdown
	// is open: then the first Escape only closes the dropdown.
	function handleDialogKeydown(event) {
		if (event.key !== 'Escape') return;
		if (event.target?.getAttribute?.('aria-expanded') === 'true') return;
		closeDialog();
	}

	async function linkSelectedPage() {
		if (!canSubmit) return;
		linking = true;
		try {
			const created = await milestonesApi.linkPage(milestoneId, selectedPage.id);
			links = [...links.filter((link) => link.id !== created.id), created];
			linking = false;
			closeDialog();
		} catch (err) {
			console.error('Failed to link page to milestone:', err);
			errorToast(err?.message || String(err), t('errors.failedToUpdate'));
		} finally {
			linking = false;
		}
	}

	async function unlinkPage(linkId) {
		try {
			await milestonesApi.unlinkPage(milestoneId, linkId);
			links = links.filter((link) => link.id !== linkId);
			// The focused remove button is gone; keep keyboard focus in the section.
			addButton?.focus();
		} catch (err) {
			console.error('Failed to unlink page from milestone:', err);
			errorToast(err?.message || String(err), t('errors.failedToUpdate'));
		}
	}
</script>

<!-- Compact list inside the milestone header card, styled like
     the item detail Pages section. Viewers only see it when pages are linked;
     editors always get the heading and "+ Add". -->
{#if visible}
	<section class="mt-6" data-testid="milestone-pages" aria-labelledby="milestone-pages-heading">
		<div class="flex items-center justify-between mb-3">
			<div class="flex items-center gap-2">
				<FileText class="w-4 h-4" style="color: var(--ds-text-subtle);" />
				<h2
					id="milestone-pages-heading"
					class="text-sm font-semibold uppercase tracking-wider"
					style="color: var(--ds-text-subtle); font-size: 11px;"
				>
					{t('items.linkedPages')}
				</h2>
			</div>
			{#if canEdit}
				<button
					type="button"
					bind:this={addButton}
					data-testid="milestone-page-add"
					class="add-page-btn inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded transition-colors cursor-pointer"
					aria-haspopup="dialog"
					onclick={openDialog}
				>
					<Plus class="w-3 h-3" />
					{t('common.add')}
				</button>
			{/if}
		</div>

		{#if error}
			<AlertBox variant="error" message={error} class="mb-3" />
		{/if}

		{#if links.length > 0}
			<ul class="space-y-2">
				{#each links as link (link.id)}
					{@const pageHref = `/workspaces/${link.workspace_id || workspaceId}/pages/${link.page_id}`}
					<li
						class="group flex items-center justify-between px-4 py-3 rounded-lg border"
						style="background-color: var(--ds-surface); border-color: var(--ds-border);"
						data-testid="milestone-page-row"
						data-link-id={link.id}
						data-page-id={link.page_id}
					>
						<div class="flex items-center gap-3 flex-1 min-w-0">
							<div
								class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
								style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
							>
								<FileText class="w-3.5 h-3.5" />
							</div>
							<LinkComponent
								href={pageHref}
								class="text-sm hover:text-ds-text-link cursor-pointer truncate"
								style="color: var(--ds-text);"
							>
								{link.page_title || t('pages.untitled')}
							</LinkComponent>
						</div>
						{#if canEdit}
							<!-- Shown on row hover and whenever focus is inside the row. -->
							<button
								type="button"
								data-testid="milestone-page-unlink"
								class="remove-page-btn p-1 rounded cursor-pointer flex-shrink-0 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100 transition-opacity"
								onclick={() => unlinkPage(link.id)}
								title={t('items.removeLink')}
								aria-label={t('items.removeLink')}
							>
								<Trash2 class="w-4 h-4" />
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}

{#if canEdit}
	<!-- Link dialog, built from the same Modal pieces as the item detail's
	     LinkItemModal in Page mode: title, page picker limited to the
	     milestone's workspace, Cancel and "Add Link". -->
	<Modal
		bind:isOpen={dialogOpen}
		maxWidth="max-w-md"
		zIndexClass="z-[60]"
		preventClose={linking}
		onclose={closeDialog}
		onSubmit={linkSelectedPage}
		submitDisabled={!canSubmit}
		dataTestid="milestone-page-link-modal"
	>
		{#snippet children(submitHint)}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div onkeydowncapture={handleDialogKeydown}>
				<ModalHeader title={t('items.addPage')} onClose={closeDialog} />

				<div class="p-6 space-y-1">
					<label
						for="milestone-page-picker"
						class="block text-sm font-medium mb-1"
						style="color: var(--ds-text-subtle);"
					>
						{t('items.targetPage')}
					</label>
					<PagePicker
						id="milestone-page-picker"
						{workspaceId}
						bind:value={selectedPageId}
						placeholder={t('items.pagePickerPlaceholder')}
						disabled={linking}
						inputTestid="milestone-page-picker"
						onSelect={handleSelectPage}
					/>
					{#if selectedAlreadyLinked}
						<p class="text-xs" style="color: var(--ds-text-subtle);" data-testid="milestone-page-already-linked">
							{t('pickers.alreadyLinked')}
						</p>
					{/if}
				</div>

				<DialogFooter
					onCancel={closeDialog}
					onConfirm={linkSelectedPage}
					confirmLabel={t('items.addPage')}
					cancelLabel={t('common.cancel')}
					disabled={!canSubmit}
					loading={linking}
					confirmKeyboardHint={submitHint}
					showKeyboardHint={true}
					confirmTestid="milestone-page-link-confirm"
					cancelTestid="milestone-page-link-cancel"
				/>
			</div>
		{/snippet}
	</Modal>
{/if}

<style>
	.add-page-btn {
		color: var(--ds-text-subtle);
	}

	.add-page-btn:hover {
		background-color: var(--ds-background-neutral-hovered);
		color: var(--ds-text);
	}

	.remove-page-btn {
		color: var(--ds-text-subtle);
	}

	.remove-page-btn:hover {
		color: var(--ds-text-danger);
	}
</style>
