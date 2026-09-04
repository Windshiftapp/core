<script>
	import { onMount, onDestroy, tick } from 'svelte';
	import { api } from '../../api.js';
	import { authStore } from '../../stores';
	import { subscribeToNewNotifications } from '../../stores/notifications.js';
	import { useEventListener } from 'runed';
	import { usePoller } from '../../composables/usePoller.svelte.js';
	import { itemLiveUpdates } from '../../stores/itemLiveUpdates.svelte.js';
	import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
	import Button from '../../components/Button.svelte';
	import SafeMarkdown from '../../components/SafeMarkdown.svelte';
	import Avatar from '../../components/Avatar.svelte';
	import Checkbox from '../../components/Checkbox.svelte';
	import { formatRelativeTime } from '../../utils/dateFormatter.js';
	import { getShortcut, matchesShortcut, getDisplayString } from '../../utils/keyboardShortcuts.js';
	import { t } from '../../stores/i18n.svelte.js';
	import { confirm } from '../../composables/useConfirm.js';
	import Badge from '../../components/Badge.svelte';
	import AlertBox from '../../components/AlertBox.svelte';
	import Tooltip from '../../components/Tooltip.svelte';
	import { Shield, Bot } from '@lucide/svelte';
	import { agentOwnerName, loadAttributedComments } from './activityAttributionData.js';
	import { isExpectedBackgroundSyncError } from '../../utils/backgroundSync.js';

	const COMMENT_PAGE_SIZE = 25;

	// Get shortcut configuration (use same as description save)
	const submitShortcut = getShortcut('description', 'save');

	let { itemId, workspaceId = null, isPersonalWorkspace = false, isPortalRequest = false, enableInternalComments = false, onCommentsLoaded } = $props();

	let comments = $state([]);
	let newCommentContent = $state('');
	let isSubmitting = $state(false);
	let error = $state('');
	let editorRef;
	let isInternalComment = $state(false);
	let hasMore = $state(false);
	let isLoadingMore = $state(false);
	let totalCount = $state(0);
	let olderCursor = $state('');
	let refreshCursor = $state('');
	let reconciliationVersion = 0;

	// Editing state
	let editingCommentId = $state(null);
	let editingContent = $state('');
	let isSavingEdit = $state(false);
	let editEditorRef = $state(null);

	// Sort state
	let sortOrder = $state('oldest'); // 'oldest' | 'newest'

	// Count of comments that arrived via polling and the user hasn't acknowledged.
	let newCount = $state(0);

	function agentTooltipContent(comment) {
		if (!comment.is_agent) return '';
		const owner = agentOwnerName(comment);
		if (owner) {
			return t('comments.agentOwnedBy', { owner });
		}
		return t('comments.agentAuthored');
	}

	const sortedComments = $derived.by(() => {
		return [...comments].sort((a, b) => {
			const dateA = new Date(a.created_at).getTime();
			const dateB = new Date(b.created_at).getTime();
			const position = dateA - dateB || a.id - b.id;
			return sortOrder === 'oldest' ? position : -position;
		});
	});

	function toggleSortOrder() {
		sortOrder = sortOrder === 'oldest' ? 'newest' : 'oldest';
	}

	function dismissNewBadge() {
		newCount = 0;
	}

	onMount(() => {
		loadComments({ initial: true });
	});

	// Poll for new comments while viewing the item. Adaptive cadence via
	// activityStore (30s active / 5m idle / hidden tab). Demoted while the item's
	// SSE stream is healthy (WI-484); resumes automatically if it drops.
	usePoller(() => pollForNewComments(), { enabled: () => !itemLiveUpdates.isLive(itemId) });

	// Instant path: a new 'comment' or 'mention' notification for the item
	// currently open triggers a refresh without waiting for the next tick.
	const unsubscribeFromBus = subscribeToNewNotifications((n) => {
		if (n.type !== 'comment' && n.type !== 'mention') return;
		if (!notificationTargetsThisItem(n)) return;
		pollForNewComments();
	});
	onDestroy(() => unsubscribeFromBus?.());

	// Reload when a notification points at the item already in view (clicking it
	// would otherwise no-op navigate()). Comments load independently of the item
	// store, so we listen for the same `reload-item-detail` event ItemDetail does
	// and self-filter on our itemId prop, refreshing without waiting for a poll.
	useEventListener(() => window, 'reload-item-detail', (/** @type {CustomEvent<{itemId?: number|string}>} */ event) => {
		const id = event?.detail?.itemId;
		if (id == null || String(id) !== String(itemId)) return;
		pollForNewComments();
	});

	// SSE comment events (WI-484) reach Comments via this lightweight event from
	// the item-detail view that owns the EventSource, so we don't open a second
	// stream. Self-filtered on itemId.
	useEventListener(() => window, 'item-comments-changed', (/** @type {CustomEvent<{itemId?: number|string}>} */ event) => {
		const id = event?.detail?.itemId;
		if (id == null || String(id) !== String(itemId)) return;
		reconcileLoadedComments();
	});

	function notificationTargetsThisItem(n) {
		const url = n.actionUrl || n.action_url || '';
		// Match /items/<id> at the end or followed by /, ?, #.
		const m = url.match(/\/items\/(\d+)(?:[/?#]|$)/);
		return m && String(itemId) === m[1];
	}

	/**
	 * Initial load of the newest comment page.
	 */
	async function loadComments({ initial = false } = {}) {
		const version = ++reconciliationVersion;
		let response;
		try {
			response = await loadAttributedComments(api, itemId, { limit: COMMENT_PAGE_SIZE });
		} catch (err) {
			if (version !== reconciliationVersion) return;
			// Navigation can unload the document while this request is in flight.
			// That is expected lifecycle control flow, not a failed comment load.
			if (isExpectedBackgroundSyncError(err)) return;
			if (initial) {
				console.error('Failed to load comments:', err);
				error = t('comments.failedToLoad');
				comments = [];
				hasMore = false;
				totalCount = 0;
				onCommentsLoaded?.({ count: 0 });
			} else {
				console.warn('Comments poll failed:', err);
			}
			return;
		}
		if (version !== reconciliationVersion) return;

		comments = response?.comments || [];
		hasMore = Boolean(response?.has_more);
		olderCursor = response?.next_cursor || '';
		refreshCursor = response?.refresh_cursor || '';
		totalCount = Number.isInteger(response?.total) ? response.total : comments.length;
		onCommentsLoaded?.({ count: totalCount });
	}

	/**
	 * Fetch only rows newer than the newest one already held. Merging by ID
	 * preserves loaded older pages, the draft box, and any in-progress editor.
	 */
	async function pollForNewComments() {
		let response;
		try {
			response = await loadAttributedComments(api, itemId, {
				limit: COMMENT_PAGE_SIZE,
				...(refreshCursor ? { cursor: refreshCursor } : {})
			});
		} catch (err) {
			console.warn('Comments poll failed:', err);
			return;
		}

		const next = response?.comments || [];
		const existingIds = new Set(comments.map((comment) => comment.id));
		const added = next.filter((comment) => !existingIds.has(comment.id));
		const hadRefreshCursor = Boolean(refreshCursor);
		refreshCursor = response?.refresh_cursor || refreshCursor;
		const currentUserId = authStore.currentUser?.id;
		const arrived = added.filter((comment) => comment.author_id !== currentUserId).length;

		comments = mergeCommentRows(comments, next);
		if (!hadRefreshCursor) {
			hasMore = Boolean(response?.has_more);
			olderCursor = response?.next_cursor || '';
			totalCount = Number.isInteger(response?.total) ? response.total : comments.length;
		} else {
			totalCount += added.length;
		}
		if (arrived > 0) newCount += arrived;
		onCommentsLoaded?.({ count: totalCount });
	}

	/**
	 * Re-fetch the same newest window the viewer has already loaded. Unlike the
	 * created-at `since` cursor, this observes edits and rows that disappeared
	 * after deletion. Paging to the current depth preserves the user's explicit
	 * Load more state without reverting to an unbounded thread fetch.
	 */
	async function reconcileLoadedComments() {
		const version = ++reconciliationVersion;
		let targetCount = Math.max(COMMENT_PAGE_SIZE, comments.length);
		const previousTotalCount = totalCount;
		const previousNewest = feedBoundary(comments, 'newest');
		const existingIds = new Set(comments.map((comment) => comment.id));
		const fresh = [];
		let cursor = '';
		let firstResponse = null;
		let lastResponse = null;

		try {
			while (fresh.length < targetCount) {
				const isFirstPage = firstResponse === null;
				const response = await loadAttributedComments(api, itemId, {
					limit: Math.min(100, targetCount - fresh.length),
					...(cursor ? { cursor } : {})
				});
				if (version !== reconciliationVersion) return;
				firstResponse ??= response;
				lastResponse = response;
				if (isFirstPage && Number.isInteger(response?.total)) {
					targetCount += Math.max(0, response.total - previousTotalCount);
				}
				const page = response?.comments || [];
				fresh.push(...page);
				if (!response?.has_more || page.length === 0) break;
				cursor = response?.next_cursor || '';
			}
		} catch (err) {
			if (version !== reconciliationVersion) return;
			console.warn('Comments reconciliation failed:', err);
			return;
		}

		const added = fresh.filter((comment) => !existingIds.has(comment.id));
		const currentUserId = authStore.currentUser?.id;
		const arrived = added.filter((comment) =>
			comment.author_id !== currentUserId &&
			(!previousNewest || compareFeedRows(comment, previousNewest) > 0)
		).length;
		comments = mergeCommentRows([], fresh);
		totalCount = Number.isInteger(firstResponse?.total) ? firstResponse.total : comments.length;
		hasMore = Boolean(lastResponse?.has_more);
		olderCursor = lastResponse?.next_cursor || '';
		refreshCursor = firstResponse?.refresh_cursor || refreshCursor;
		if (editingCommentId && !comments.some((comment) => comment.id === editingCommentId)) {
			editingCommentId = null;
			editingContent = '';
		}
		if (arrived > 0) newCount += arrived;
		onCommentsLoaded?.({ count: totalCount });
	}

	async function loadMoreComments() {
		if (isLoadingMore || !hasMore) return;
		if (!olderCursor) return;

		isLoadingMore = true;
		error = '';
		try {
			const response = await loadAttributedComments(api, itemId, {
				limit: COMMENT_PAGE_SIZE,
				cursor: olderCursor
			});
			comments = mergeCommentRows(comments, response?.comments || []);
			hasMore = Boolean(response?.has_more);
			olderCursor = response?.next_cursor || '';
			onCommentsLoaded?.({ count: totalCount });
		} catch (err) {
			console.error('Failed to load older comments:', err);
			error = t('comments.failedToLoad');
		} finally {
			isLoadingMore = false;
		}
	}

	function mergeCommentRows(current, incoming) {
		const byId = new Map(current.map((comment) => [comment.id, comment]));
		for (const comment of incoming) byId.set(comment.id, comment);
		return [...byId.values()];
	}

	function feedBoundary(rows, direction) {
		if (rows.length === 0) return null;
		return rows.reduce((boundary, candidate) => {
			const comparison = compareFeedRows(candidate, boundary);
			if (direction === 'newest' ? comparison > 0 : comparison < 0) return candidate;
			return boundary;
		});
	}

	function compareFeedRows(left, right) {
		const leftTime = new Date(left.created_at).getTime();
		const rightTime = new Date(right.created_at).getTime();
		return leftTime - rightTime || left.id - right.id;
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

			const alreadyLoaded = comments.some((comment) => comment.id === newComment.id);
			comments = mergeCommentRows(comments, [newComment]);
			if (!alreadyLoaded) totalCount++;
			newCommentContent = '';
			editorRef?.clear();
			isInternalComment = false; // Reset toggle after posting
			newCount = 0; // User engaged — clear the "new comments" badge.
			// Update comment count
			onCommentsLoaded?.({ count: totalCount });
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
		}
		// Escape intentionally not handled here — let it bubble so the
		// enclosing modal (or page-level handler) can close as expected.
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
			totalCount = Math.max(0, totalCount - 1);
			// Update comment count
			onCommentsLoaded?.({ count: totalCount });
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
					? {
							...c,
							content: updated.content,
							content_html: updated.content_html,
							updated_at: updated.updated_at
						}
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

<div class="comments-section" data-testid="comments-section">
	{#if error}
		<div data-testid="comments-error">
			<AlertBox variant="error" message={error} class="mb-4" />
		</div>
	{/if}

	<!-- Sort Toggle + new-comments badge -->
	{#if comments.length > 1 || newCount > 0}
		<div class="flex items-center justify-end gap-2 mb-4">
			{#if newCount > 0}
				<button
					onclick={dismissNewBadge}
					data-testid="new-comments-badge"
					data-new-count={newCount}
					class="flex items-center gap-1.5 text-xs px-2 py-1 rounded transition-colors hover:opacity-80"
					style="background: var(--ds-info-bg, #dbeafe); color: var(--ds-info-text, #1e40af);"
					title={t('common.close')}
				>
					<span class="w-1.5 h-1.5 rounded-full" style="background: currentColor;"></span>
					{t('comments.newCommentsAvailable', { count: newCount })}
				</button>
			{/if}
			{#if comments.length > 1}
				<button
					onclick={toggleSortOrder}
					data-testid="comments-sort-toggle"
					class="flex items-center gap-1.5 text-xs px-2 py-1 rounded transition-colors hover:bg-[var(--ds-bg-subtle)]"
					style="color: var(--ds-text-subtle);"
				>
					<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"></path>
					</svg>
					{sortOrder === 'oldest' ? t('comments.oldestFirst') : t('comments.newestFirst')}
				</button>
			{/if}
		</div>
	{/if}

	{#if hasMore}
		<div class="flex justify-center mb-4">
			<Button
				variant="secondary"
				size="small"
				dataTestid="comments-load-more"
				onclick={loadMoreComments}
				disabled={isLoadingMore}
			>
				{isLoadingMore ? t('common.loading') : t('common.loadMore')}
			</Button>
		</div>
	{/if}

	<!-- Comments List -->
	<div class="space-y-4">
		{#each sortedComments as comment (comment.id)}
			<div class="flex items-start space-x-3 group" data-testid="comment-item" data-comment-id={comment.id}>
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
								{#if comment.source === 'approval'}
									<Tooltip content="Posted as part of an approval decision" placement="top">
										<Shield class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
									</Tooltip>
								{:else if comment.is_agent}
									<Tooltip content={agentTooltipContent(comment)} placement="top">
										<Bot class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
									</Tooltip>
								{/if}
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
									<Badge variant="warning" size="xs" class="uppercase">{t('comments.internal')}</Badge>
								{/if}
							</div>
							{#if comment.source !== 'approval' && authStore.currentUser && comment.author_id === authStore.currentUser.id && editingCommentId !== comment.id}
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
									{workspaceId}
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
								<SafeMarkdown html={comment.content_html} source={comment.content} compact={true} />
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
				<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
				<div data-testid="comment-editor" onclick={() => editorRef?.focus()}>
					<MilkdownEditor
						bind:this={editorRef}
						bind:content={newCommentContent}
						placeholder={t('comments.writePlaceholder')}
						showToolbar={true}
						hideToolbarUntilFocus={true}
						compact={true}
						testId="comment-composer"
						{itemId}
						{workspaceId}
						{isPersonalWorkspace}
					/>
				</div>
				<div class="flex items-center justify-between mt-3">
					<div class="flex items-center gap-4">
						<div class="text-xs" style="color: var(--ds-text-subtle);">
							{t('comments.markdownSupported')}
						</div>
						{#if isPortalRequest || enableInternalComments}
							<Checkbox
								bind:checked={isInternalComment}
								label={t('comments.internalNote')}
								hint={isPortalRequest ? t('comments.internalNoteHint') : t('comments.internalNoteHintGeneral')}
								size="small"
							/>
						{/if}
					</div>
					<!-- shortcut-guard-exempt: Cmd/Ctrl+Enter is handled by the form-scoped handleCommentKeydown handler. -->
					<Button
						variant="primary"
						size="small"
						dataTestid="comment-submit"
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
