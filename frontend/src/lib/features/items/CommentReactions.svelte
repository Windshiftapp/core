<script>
	// Emoji reactions under a comment (core#298): a quick way to acknowledge a
	// comment without writing a reply. Clicking a chip toggles the viewer's own
	// reaction; the picker offers the fixed server-side reaction set.
	import SmilePlus from '@lucide/svelte/icons/smile-plus';
	import { useEventListener } from 'runed';
	import { addCommentReaction, removeCommentReaction } from '../../api/misc.js';
	import { authStore } from '../../stores/auth.svelte.js';
	import { t } from '../../stores/i18n.svelte.js';
	import Tooltip from '../../components/Tooltip.svelte';

	let { comment, canReact = false, onChange, onError } = $props();

	const REACTIONS = [
		{ key: 'thumbs_up', emoji: '👍', label: 'comments.reactionThumbsUp' },
		{ key: 'check', emoji: '✅', label: 'comments.reactionCheck' },
		{ key: 'eyes', emoji: '👀', label: 'comments.reactionEyes' },
		{ key: 'heart', emoji: '❤️', label: 'comments.reactionHeart' },
		{ key: 'laugh', emoji: '😄', label: 'comments.reactionLaugh' },
		{ key: 'hooray', emoji: '🎉', label: 'comments.reactionHooray' }
	];
	const byKey = new Map(REACTIONS.map((reaction) => [reaction.key, reaction]));

	let pickerOpen = $state(false);
	let busy = $state(false);
	let root = $state(null);

	const reactions = $derived((comment.reactions || []).filter((group) => byKey.has(group.reaction) && group.count > 0));
	const currentUserId = $derived(authStore.currentUser?.id);

	function mine(group) {
		return currentUserId != null && (group.user_ids || []).includes(currentUserId);
	}

	function tooltip(group) {
		const reaction = byKey.get(group.reaction);
		return t('comments.reactedBy', { reaction: `${reaction.emoji} ${t(reaction.label)}`, names: (group.user_names || []).join(', ') });
	}

	// Local result of a toggle, so removal needs no extra round trip.
	function withToggle(groups, key, add) {
		const user = authStore.currentUser;
		const name = [user?.first_name, user?.last_name].filter(Boolean).join(' ') || user?.username || '';
		const next = groups.map((group) => ({ ...group, user_ids: [...(group.user_ids || [])], user_names: [...(group.user_names || [])] }));
		let group = next.find((candidate) => candidate.reaction === key);
		if (add) {
			if (!group) {
				group = { reaction: key, count: 0, user_ids: [], user_names: [] };
				next.push(group);
			}
			group.user_ids.push(user.id);
			group.user_names.push(name);
		} else if (group) {
			const index = group.user_ids.indexOf(user.id);
			if (index >= 0) {
				group.user_ids.splice(index, 1);
				group.user_names.splice(index, 1);
			}
		}
		for (const candidate of next) candidate.count = candidate.user_ids.length;
		const order = REACTIONS.map((reaction) => reaction.key);
		return next.filter((candidate) => candidate.count > 0).sort((a, b) => order.indexOf(a.reaction) - order.indexOf(b.reaction));
	}

	async function toggle(key) {
		if (!canReact || busy || !authStore.currentUser) return;
		pickerOpen = false;
		const previous = comment.reactions || [];
		const add = !previous.some((group) => group.reaction === key && mine(group));
		busy = true;
		onChange?.(withToggle(previous, key, add));
		try {
			if (add) {
				const updated = await addCommentReaction(comment.id, key);
				if (Array.isArray(updated)) onChange?.(updated);
			} else {
				await removeCommentReaction(comment.id, key);
			}
		} catch (err) {
			console.error('Failed to update reaction:', err);
			onChange?.(previous);
			onError?.(t('comments.reactionFailed'));
		} finally {
			busy = false;
		}
	}

	useEventListener(() => window, 'pointerdown', (event) => {
		if (pickerOpen && root && !root.contains(event.target)) pickerOpen = false;
	});
	useEventListener(() => window, 'keydown', (event) => {
		if (pickerOpen && event.key === 'Escape') {
			event.stopPropagation();
			pickerOpen = false;
		}
	});
</script>

{#if reactions.length > 0 || canReact}
	<div bind:this={root} class="relative mt-1.5 flex flex-wrap items-center gap-1" data-testid="comment-reactions">
		{#each reactions as group (group.reaction)}
			<Tooltip content={tooltip(group)} placement="top">
				<button
					type="button"
					onclick={() => toggle(group.reaction)}
					disabled={!canReact || busy}
					data-testid="comment-reaction"
					data-reaction={group.reaction}
					aria-pressed={mine(group)}
					class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors disabled:cursor-default"
					style={mine(group)
						? 'border-color: var(--ds-interactive); background: var(--ds-interactive-subtle); color: var(--ds-interactive);'
						: 'border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);'}
				>
					<span aria-hidden="true">{byKey.get(group.reaction).emoji}</span>
					<span>{group.count}</span>
				</button>
			</Tooltip>
		{/each}
		{#if canReact}
			<button
				type="button"
				onclick={() => (pickerOpen = !pickerOpen)}
				disabled={busy}
				data-testid="comment-reaction-add"
				aria-expanded={pickerOpen}
				aria-label={t('comments.addReaction')}
				title={t('comments.addReaction')}
				class="inline-flex items-center rounded-full px-1.5 py-0.5 transition-opacity hover:bg-[var(--ds-background-neutral-hovered)] {reactions.length > 0 || pickerOpen ? '' : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100'}"
				style="color: var(--ds-text-subtle);"
			>
				<SmilePlus class="w-4 h-4" />
			</button>
			{#if pickerOpen}
				<div
					role="menu"
					class="absolute left-0 top-full z-20 mt-1 flex gap-0.5 rounded-lg border p-1"
					style="border-color: var(--ds-border); background: var(--ds-surface-raised); box-shadow: var(--ds-shadow-raised);"
					data-testid="comment-reaction-picker"
				>
					{#each REACTIONS as reaction (reaction.key)}
						<button
							type="button"
							role="menuitem"
							onclick={() => toggle(reaction.key)}
							data-testid="comment-reaction-option"
							data-reaction={reaction.key}
							title={t(reaction.label)}
							aria-label={t(reaction.label)}
							class="rounded px-1.5 py-0.5 text-base hover:bg-[var(--ds-background-neutral-hovered)]"
						>
							{reaction.emoji}
						</button>
					{/each}
				</div>
			{/if}
		{/if}
	</div>
{/if}
