<script>
	import { onMount, createEventDispatcher, tick } from 'svelte';
	import { api } from '../../api.js';
	import { authStore } from '../../stores';
	import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
	import Button from '../../components/Button.svelte';
	import Avatar from '../../components/Avatar.svelte';
	import { formatRelativeTime } from '../../utils/dateFormatter.js';
	import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
	import { t } from '../../stores/i18n.svelte.js';

	// Get shortcut configuration (use same as description save)
	const submitShortcut = getShortcut('description', 'save');

	export let itemId;
	export let isPersonalWorkspace = false;

	const dispatch = createEventDispatcher();

	let comments = [];
	let newCommentContent = '';
	let isSubmitting = false;
	let error = '';
	let editorRef;

	// Editing state
	let editingCommentId = null;
	let editingContent = '';
	let isSavingEdit = false;
	let editEditorRef;

	onMount(() => {
		loadComments();
	});

	async function loadComments() {
		try {
			comments = await api.getComments(itemId) || [];
			// Dispatch event to update comment count in parent
			dispatch('commentsLoaded', { count: comments.length });
		} catch (err) {
			console.error('Failed to load comments:', err);
			error = t('comments.failedToLoad');
			comments = [];
			dispatch('commentsLoaded', { count: 0 });
		}
	}

	async function submitComment() {
		if (!newCommentContent.trim() || !authStore.currentUser) return;

		isSubmitting = true;
		error = '';

		try {
			const newComment = await api.createComment(itemId, {
				content: newCommentContent,
				author_id: authStore.currentUser.id
			});

			comments = [...comments, newComment];
			newCommentContent = '';
			editorRef?.clear();
			// Update comment count
			dispatch('commentsLoaded', { count: comments.length });
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
		if (!confirm(t('comments.confirmDelete'))) return;

		try {
			await api.deleteComment(commentId);
			comments = comments.filter(c => c.id !== commentId);
			// Update comment count
			dispatch('commentsLoaded', { count: comments.length });
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

	<!-- Comments List -->
	<div class="space-y-4">
		{#each comments as comment (comment.id)}
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
			<div class="flex-1" onkeydown={handleCommentKeydown}>
				<MilkdownEditor
					bind:this={editorRef}
					bind:content={newCommentContent}
					placeholder={t('comments.writePlaceholder')}
					showToolbar={true}
					compact={true}
					{itemId}
					{isPersonalWorkspace}
				/>
				<div class="flex items-center justify-between mt-3">
					<div class="text-xs" style="color: var(--ds-text-subtle);">
						{t('comments.markdownSupported')}
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

