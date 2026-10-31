<script>
	import { onMount, tick } from 'svelte';
	import { milestones as milestonesApi } from '../../api/milestones.js';
	import { authStore } from '../../stores/auth.svelte.js';
	import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
	import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
	import Button from '../../components/Button.svelte';
	import Avatar from '../../components/Avatar.svelte';
	import AlertBox from '../../components/AlertBox.svelte';
	import StateDisplay from '../../components/StateDisplay.svelte';
	import { formatRelativeTime } from '../../utils/dateFormatter.js';
	import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
	import { t } from '../../stores/i18n.svelte.js';
	import { confirm } from '../../composables/useConfirm.js';
	import { newestCommentsFirst } from './milestoneCommentOrder.js';

	// Milestone comments. Local milestones apply their workspace's item
	// comment permissions; on global ones any viewer comments and authors
	// change their own (mirrors MilestoneCommentService). Unlike item comments these
	// send no notifications, so @mentions notify nobody, and milestones have
	// no attachments, so image upload is off in every editor here.
	// The thread shows newest first under the composer. It pages like the
	// item comment feed: the newest page first, then Load more follows
	// next_cursor to older comments.
	let { milestoneId, workspaceId = null } = $props();

	const submitShortcut = getShortcut('description', 'save');
	const COMMENT_PAGE_SIZE = 25;

	let comments = $state([]);
	let loading = $state(true);
	let error = $state('');
	let hasMore = $state(false);
	let isLoadingMore = $state(false);
	let olderCursor = $state('');
	let totalCount = $state(0);
	let newCommentContent = $state('');
	let isSubmitting = $state(false);
	let editorRef = $state(null);

	let editingCommentId = $state(null);
	let editingContent = $state('');
	let isSavingEdit = $state(false);
	let editEditorRef = $state(null);

	const currentUserId = $derived(authStore.currentUser?.id ?? null);
	// comment.edit_others holders may moderate foreign comments on local
	// milestones. Global milestones (workspaceId null) have no workspace.
	const canEditOthersComments = $derived(
		workspaceId != null && workspacePermissions.canEditOthersComments(workspaceId)
	);
	// Loaded pages plus comments posted here, newest first.
	const orderedComments = $derived(newestCommentsFirst(comments));

	onMount(() => {
		loadComments();
	});

	async function loadComments() {
		loading = true;
		error = '';
		try {
			const response = await milestonesApi.getComments(milestoneId, { limit: COMMENT_PAGE_SIZE });
			comments = response?.comments || [];
			hasMore = Boolean(response?.has_more);
			olderCursor = response?.next_cursor || '';
			totalCount = Number.isInteger(response?.total) ? response.total : comments.length;
		} catch (err) {
			console.error('Failed to load milestone comments:', err);
			error = t('comments.failedToLoad');
			comments = [];
			hasMore = false;
			totalCount = 0;
		} finally {
			loading = false;
		}
	}

	async function loadMoreComments() {
		if (isLoadingMore || !hasMore || !olderCursor) return;
		isLoadingMore = true;
		error = '';
		try {
			const response = await milestonesApi.getComments(milestoneId, {
				limit: COMMENT_PAGE_SIZE,
				cursor: olderCursor
			});
			const seen = new Set(comments.map((comment) => comment.id));
			comments = [...comments, ...(response?.comments || []).filter((comment) => !seen.has(comment.id))];
			hasMore = Boolean(response?.has_more);
			olderCursor = response?.next_cursor || '';
		} catch (err) {
			console.error('Failed to load older milestone comments:', err);
			error = t('comments.failedToLoad');
		} finally {
			isLoadingMore = false;
		}
	}

	async function submitComment() {
		if (!newCommentContent.trim() || !authStore.currentUser || isSubmitting) return;
		isSubmitting = true;
		error = '';
		try {
			const created = await milestonesApi.createComment(milestoneId, newCommentContent);
			if (!comments.some((comment) => comment.id === created.id)) totalCount += 1;
			comments = [...comments.filter((comment) => comment.id !== created.id), created];
			newCommentContent = '';
			editorRef?.clear();
		} catch (err) {
			console.error('Failed to create milestone comment:', err);
			error = t('comments.failedToCreate');
		} finally {
			isSubmitting = false;
		}
	}

	function handleCommentKeydown(event) {
		if (matchesShortcut(event, submitShortcut)) {
			event.preventDefault();
			submitComment();
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
			await milestonesApi.deleteComment(milestoneId, commentId);
			comments = comments.filter((comment) => comment.id !== commentId);
			totalCount = Math.max(0, totalCount - 1);
		} catch (err) {
			console.error('Failed to delete milestone comment:', err);
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
		if (!editingContent.trim() || !editingCommentId || isSavingEdit) return;
		isSavingEdit = true;
		error = '';
		try {
			const updated = await milestonesApi.updateComment(milestoneId, editingCommentId, editingContent);
			comments = comments.map((comment) => (comment.id === updated.id ? updated : comment));
			cancelEdit();
		} catch (err) {
			console.error('Failed to update milestone comment:', err);
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
		return new Date(comment.updated_at).getTime() - new Date(comment.created_at).getTime() > 1000;
	}
</script>

<section
	class="rounded-xl border p-6 mt-6"
	style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
	data-testid="milestone-comments"
>
	<h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
		{t('common.comments')}
		{#if totalCount > 0}
			<span class="text-sm font-normal" style="color: var(--ds-text-subtle);">({totalCount})</span>
		{/if}
	</h2>

	{#if error}
		<AlertBox variant="error" message={error} class="mb-4" />
	{/if}

	{#if loading}
		<StateDisplay type="loading" />
	{:else}
		{#if authStore.currentUser}
			<div class="mb-6 flex items-start space-x-3">
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
				<div class="flex-1 min-w-0" onkeydown={handleCommentKeydown}>
					<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
					<div data-testid="milestone-comment-editor" onclick={() => editorRef?.focus()}>
						<MilkdownEditor
							bind:this={editorRef}
							bind:content={newCommentContent}
							placeholder={t('comments.writePlaceholder')}
							showToolbar={true}
							hideToolbarUntilFocus={true}
							compact={true}
							allowImageUpload={false}
							testId="milestone-comment-composer"
							{workspaceId}
						/>
					</div>
					<div class="flex items-center justify-between mt-3">
						<div class="text-xs" style="color: var(--ds-text-subtle);">
							{t('comments.markdownSupported')}
						</div>
						<!-- shortcut-guard-exempt: Cmd/Ctrl+Enter is handled by the form-scoped handleCommentKeydown handler. -->
						<Button
							variant="primary"
							size="small"
							dataTestid="milestone-comment-submit"
							onclick={submitComment}
							disabled={isSubmitting || !newCommentContent.trim()}
							keyboardHint={getDisplayString(submitShortcut)}
						>
							{isSubmitting ? t('comments.posting') : t('comments.comment')}
						</Button>
					</div>
				</div>
			</div>
		{/if}

		{#if comments.length === 0}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('comments.noComments')}</p>
		{/if}

		<div class="space-y-4">
			{#each orderedComments as comment (comment.id)}
				<div class="flex items-start space-x-3 group" data-testid="milestone-comment" data-comment-id={comment.id}>
					<div class="flex-shrink-0">
						<Avatar src={comment.author_avatar} name={comment.author_name} size="sm" variant="neutral" />
					</div>
					<div class="flex-1 min-w-0">
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
							{#if currentUserId != null && (comment.author_id === currentUserId || canEditOthersComments) && editingCommentId !== comment.id}
								<div class="flex items-center space-x-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
									<button
										type="button"
										onclick={() => startEdit(comment)}
										data-testid="milestone-comment-edit"
										class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
										title={t('comments.editComment')}
										aria-label={t('comments.editComment')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
										</svg>
									</button>
									<button
										type="button"
										onclick={() => deleteComment(comment.id)}
										data-testid="milestone-comment-delete"
										class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-danger)] transition-colors"
										title={t('comments.deleteComment')}
										aria-label={t('comments.deleteComment')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
										</svg>
									</button>
								</div>
							{/if}
						</div>
						{#if editingCommentId === comment.id}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div onkeydown={handleEditKeydown}>
								<MilkdownEditor
									bind:this={editEditorRef}
									bind:content={editingContent}
									placeholder={t('comments.editPlaceholder')}
									showToolbar={true}
									compact={true}
									allowImageUpload={false}
									{workspaceId}
								/>
								<div class="flex items-center justify-between mt-3">
									<div class="text-xs" style="color: var(--ds-text-subtle);">
										{t('common.pressEscapeToCancel')}
									</div>
									<div class="flex items-center space-x-2">
										<Button variant="secondary" size="small" onclick={cancelEdit} disabled={isSavingEdit}>
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
							<div class="text-sm" style="color: var(--ds-text);">
								<MilkdownEditor content={comment.content} readonly={true} showToolbar={false} compact={true} />
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>

		{#if hasMore}
			<div class="flex justify-center mt-4">
				<Button
					variant="secondary"
					size="small"
					dataTestid="milestone-comments-load-more"
					onclick={loadMoreComments}
					disabled={isLoadingMore}
				>
					{isLoadingMore ? t('common.loading') : t('common.loadMore')}
				</Button>
			</div>
		{/if}
	{/if}
</section>
