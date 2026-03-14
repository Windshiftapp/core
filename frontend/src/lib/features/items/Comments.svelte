<script>
	import { onMount, tick } from 'svelte';
	import { api } from '../../api.js';
	import { authStore } from '../../stores';
	import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
	import Button from '../../components/Button.svelte';
	import Avatar from '../../components/Avatar.svelte';
	import Checkbox from '../../components/Checkbox.svelte';
	import { formatRelativeTime } from '../../utils/dateFormatter.js';
	import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
	import { t } from '../../stores/i18n.svelte.js';
	import { confirm } from '../../composables/useConfirm.js';

	// Get shortcut configuration (use same as description save)
	const submitShortcut = getShortcut('description', 'save');

	let { itemId, isPersonalWorkspace = false, isPortalRequest = false, onCommentsLoaded } = $props();

	let comments = $state([]);
	let newCommentContent = $state('');
	let isSubmitting = $state(false);
	let error = $state('');
	let editorRef;
	let isInternalComment = $state(false);

	// Editing state
	let editingCommentId = $state(null);
	let editingContent = $state('');
	let isSavingEdit = $state(false);
	let editEditorRef = $state(null);

	// Sort state
	let sortOrder = $state('oldest'); // 'oldest' | 'newest'

	const sortedComments = $derived.by(() => {
		return [...comments].sort((a, b) => {
			const dateA = new Date(a.created_at).getTime();
			const dateB = new Date(b.created_at).getTime();
			return sortOrder === 'oldest' ? dateA - dateB : dateB - dateA;
		});
	});

	function toggleSortOrder() {
		sortOrder = sortOrder === 'oldest' ? 'newest' : 'oldest';
	}

	onMount(() => {
		loadComments();
	});

	async function loadComments() {
		try {
			comments = await api.getComments(itemId) || [];
			// Notify parent of comment count
			onCommentsLoaded?.({ count: comments.length });
		} catch (err) {
			console.error('Failed to load comments:', err);
			error = t('comments.failedToLoad');
			comments = [];
			onCommentsLoaded?.({ count: 0 });
		}
	}

	async function submitComment() {
		if (!newCommentContent.trim() || !authStore.currentUser) return;

		isSubmitting = true;
		error = '';

		try {
			const newComment = await api.createComment(itemId, {
				content: newCommentContent,
				author_id: authStore.currentUser.id,
				is_private: isInternalComment
			});

			comments = [...comments, newComment];
			newCommentContent = '';
			editorRef?.clear();
			isInternalComment = false; // Reset toggle after posting
			// Update comment count
			onCommentsLoaded?.({ count: comments.length });
		} catch (err) {
			console.error('Failed to create comment:', err);
			error = t('comments.failedToCreate');
		} finally {
			isSubmitting = false;
		}
	}

	function handleCommentKeydown(event) {
		// Check for save shortcut (Ctrl/Cmd+Enter)
		if (matchesShortcut(event, submitShortcut)) {
			event.preventDefault();
			submitComment();
		} else if (event.key === 'Escape') {
			event.preventDefault();
			editorRef?.blur?.();
		}
	}

	async function deleteComment(commentId) {
		const confirmed = await confirm({
			title: t('common.delete'),
			message: t('comments.confirmDelete'),
			confirmText: t('common.delete'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!confirmed) return;

		try {
			await api.deleteComment(commentId);
			comments = comments.filter(c => c.id !== commentId);
			// Update comment count
			onCommentsLoaded?.({ count: comments.length });
		} catch (err) {
			console.error('Failed to delete comment:', err);
			error = t('comments.failedToDelete');
		}
	}

	async function startEdit(comment) {
		editingCommentId = comment.id;
		editingContent = comment.content;
		await tick();
		editEditorRef?.focusEnd();
	}

	function cancelEdit() {
		editingCommentId = null;
		editingContent = '';
	}

	async function saveEdit() {
		if (!editingContent.trim() || !editingCommentId) return;

		isSavingEdit = true;
		error = '';

		try {
			const updated = await api.updateComment(editingCommentId, {
				content: editingContent
			});

			// Update local state
			comments = comments.map(c =>
				c.id === editingCommentId
					? { ...c, content: updated.content, updated_at: updated.updated_at }
					: c
			);

			editingCommentId = null;
			editingContent = '';
		} catch (err) {
			console.error('Failed to update comment:', err);
			error = t('comments.failedToUpdate');
		} finally {
			isSavingEdit = false;
		}
	}

	function handleEditKeydown(event) {
		if (matchesShortcut(event, submitShortcut)) {
			event.preventDefault();
			saveEdit();
		} else if (event.key === 'Escape') {
			event.preventDefault();
			cancelEdit();
		}
	}

	function isEdited(comment) {
		if (!comment.updated_at || !comment.created_at) return false;
		const created = new Date(comment.created_at).getTime();
		const updated = new Date(comment.updated_at).getTime();
		// Consider edited if updated more than 1 second after creation
		return updated - created > 1000;
	}
</script>

<div class="comments-section">
	{#if error}
		<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-md">
			<p class="text-red-700 text-sm">{error}</p>
		</div>
	{/if}

	<!-- Sort Toggle -->
	{#if comments.length > 1}
		<div class="flex items-center justify-end mb-4">
			<button
				onclick={toggleSortOrder}
				class="flex items-center gap-1.5 text-xs px-2 py-1 rounded transition-colors hover:bg-[var(--ds-bg-subtle)]"
				style="color: var(--ds-text-subtle);"
			>
				<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"></path>
				</svg>
				{sortOrder === 'oldest' ? t('comments.oldestFirst') : t('comments.newestFirst')}
			</button>
		</div>
	{/if}

	<!-- Comments List -->
	<div class="space-y-4">
		{#each sortedComments as comment (comment.id)}
			<div class="flex items-start space-x-3 group">
				<div class="flex-shrink-0">
					<Avatar
						src={comment.author_avatar}
						name={comment.author_name}
						size="sm"
						variant="neutral"
					/>
				</div>
				<div class="flex-1 min-w-0">
					<div class="py-1">
						<div class="flex items-center justify-between mb-2">
							<div class="flex items-center space-x-2">
								<h4 class="text-sm font-medium" style="color: var(--ds-text);">
									{comment.author_name || t('common.unknownUser')}
								</h4>
								<span class="text-xs" style="color: var(--ds-text-subtle);">
									{formatRelativeTime(comment.created_at) || '-'}
								</span>
								{#if isEdited(comment)}
									<span class="text-xs" style="color: var(--ds-text-subtlest);">({t('comments.edited')})</span>
								{/if}
								{#if comment.is_private}
									<span class="internal-badge">{t('comments.internal')}</span>
								{/if}
							</div>
							{#if authStore.currentUser && comment.author_id === authStore.currentUser.id && editingCommentId !== comment.id}
								<div class="flex items-center space-x-1 opacity-0 group-hover:opacity-100 transition-opacity">
									<button
										onclick={() => startEdit(comment)}
										class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
										title={t('comments.editComment')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
										</svg>
									</button>
									<button
										onclick={() => deleteComment(comment.id)}
										class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-danger)] transition-colors"
										title={t('comments.deleteComment')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
										</svg>
									</button>
								</div>
							{/if}
						</div>
						{#if editingCommentId === comment.id}
							<!-- Edit Mode -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div onkeydown={handleEditKeydown}>
								<MilkdownEditor
									bind:this={editEditorRef}
									bind:content={editingContent}
									placeholder={t('comments.editPlaceholder')}
									showToolbar={true}
									compact={true}
									{itemId}
									{isPersonalWorkspace}
								/>
								<div class="flex items-center justify-between mt-3">
									<div class="text-xs" style="color: var(--ds-text-subtle);">
										{t('common.pressEscapeToCancel')}
									</div>
									<div class="flex items-center space-x-2">
										<Button
											variant="secondary"
											size="small"
											onclick={cancelEdit}
											disabled={isSavingEdit}
										>
											{t('common.cancel')}
										</Button>
										<Button
											variant="primary"
											size="small"
											onclick={saveEdit}
											disabled={isSavingEdit || !editingContent.trim()}
											keyboardHint={getDisplayString(submitShortcut)}
										>
											{isSavingEdit ? t('common.saving') : t('common.save')}
										</Button>
									</div>
								</div>
							</div>
						{:else}
							<!-- Read Mode -->
							<div class="comment-content text-sm" style="color: var(--ds-text);">
								<MilkdownEditor
									content={comment.content}
									readonly={true}
									showToolbar={false}
								/>
							</div>
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- Comment Form -->
	<div class="mt-6">
		<div class="flex items-start space-x-3">
			<div class="flex-shrink-0">
				<Avatar
					src={authStore.currentUser?.avatar_url}
					firstName={authStore.currentUser?.first_name}
					lastName={authStore.currentUser?.last_name}
					size="sm"
					variant="blue"
				/>
			</div>
			<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="flex-1" onkeydown={handleCommentKeydown}>
				<MilkdownEditor
					bind:this={editorRef}
					bind:content={newCommentContent}
					placeholder={t('comments.writePlaceholder')}
					showToolbar={true}
					hideToolbarUntilFocus={true}
					compact={true}
					{itemId}
					{isPersonalWorkspace}
				/>
				<div class="flex items-center justify-between mt-3">
					<div class="flex items-center gap-4">
						<div class="text-xs" style="color: var(--ds-text-subtle);">
							{t('comments.markdownSupported')}
						</div>
						{#if isPortalRequest}
							<Checkbox
								bind:checked={isInternalComment}
								label={t('comments.internalNote')}
								hint={t('comments.internalNoteHint')}
								size="small"
							/>
						{/if}
					</div>
					<Button
						variant="primary"
						size="small"
						onclick={submitComment}
						disabled={isSubmitting || !newCommentContent.trim()}
						keyboardHint={getDisplayString(submitShortcut)}
					>
						{isSubmitting ? t('comments.posting') : t('comments.comment')}
					</Button>
				</div>
			</div>
		</div>
	</div>
</div>

<style>
	.internal-badge {
		display: inline-flex;
		align-items: center;
		padding: 2px 6px;
		font-size: 10px;
		font-weight: 500;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		background-color: var(--ds-status-warning-bg);
		color: var(--ds-status-warning-text);
		border-radius: 4px;
	}
</style>
