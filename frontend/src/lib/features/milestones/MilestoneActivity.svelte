<script>
	import {
		IconMessage,
		IconPencil,
		IconFlag,
		IconCalendar,
		IconFileText,
		IconFileOff,
		IconProgress,
		IconCirclePlus,
		IconCircleMinus,
		IconHistory
	} from '@tabler/icons-svelte-runes';
	import { milestones as milestonesApi } from '../../api/milestones.js';
	import AlertBox from '../../components/AlertBox.svelte';
	import Button from '../../components/Button.svelte';
	import StateDisplay from '../../components/StateDisplay.svelte';
	import LinkComponent from '../../components/Link.svelte';
	import { formatDateShort, formatDateTimeLocale, formatRelativeTime } from '../../utils/dateFormatter.js';
	import { t } from '../../stores/i18n.svelte.js';
	import { describeMilestoneActivity } from './milestoneActivity.js';

	// Milestone Activity feed: one shared, newest-first feed of
	// milestone events and member-item events. The server limits item events
	// to items the viewer can access. reloadToken changes when the milestone
	// itself is edited so the feed picks up the new entries.
	let { milestoneId, reloadToken = 0 } = $props();

	const PAGE_SIZE = 50;
	const ICONS = {
		milestone_comment_added: IconMessage,
		milestone_description_changed: IconPencil,
		milestone_status_changed: IconFlag,
		milestone_target_date_changed: IconCalendar,
		milestone_page_linked: IconFileText,
		milestone_page_unlinked: IconFileOff,
		item_comment_added: IconMessage,
		item_status_changed: IconProgress,
		item_added: IconCirclePlus,
		item_removed: IconCircleMinus
	};

	let entries = $state([]);
	let page = $state(0);
	let hasMore = $state(false);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');
	// Each first-page load starts a new feed. Responses that arrive after a
	// newer load started belong to the old feed and are dropped.
	let feedVersion = 0;

	$effect(() => {
		// Track the inputs that require a fresh first page.
		void milestoneId;
		void reloadToken;
		loadFirstPage();
	});

	async function loadFirstPage() {
		const version = ++feedVersion;
		loading = true;
		loadingMore = false;
		error = '';
		try {
			const result = await milestonesApi.getActivity(milestoneId, { page: 1, pageSize: PAGE_SIZE });
			if (version !== feedVersion) return;
			entries = result.entries;
			page = 1;
			hasMore = result.hasMore;
		} catch (err) {
			if (version !== feedVersion) return;
			console.error('Failed to load milestone activity:', err);
			error = t('dialogs.alerts.failedToLoad', { error: t('milestones.activity.tabActivity') });
			entries = [];
		} finally {
			if (version === feedVersion) loading = false;
		}
	}

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		const version = feedVersion;
		loadingMore = true;
		error = '';
		try {
			const result = await milestonesApi.getActivity(milestoneId, { page: page + 1, pageSize: PAGE_SIZE });
			if (version !== feedVersion) return;
			const seen = new Set(entries.map((entry) => entry.id));
			entries = [...entries, ...result.entries.filter((entry) => !seen.has(entry.id))];
			page += 1;
			hasMore = result.hasMore;
		} catch (err) {
			if (version !== feedVersion) return;
			console.error('Failed to load more milestone activity:', err);
			error = t('dialogs.alerts.failedToLoad', { error: t('milestones.activity.tabActivity') });
		} finally {
			if (version === feedVersion) loadingMore = false;
		}
	}
</script>

<section
	class="rounded-xl border p-6"
	style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
	data-testid="milestone-activity"
>
	{#if error}
		<AlertBox variant="error" message={error} class="mb-4" />
	{/if}

	{#if loading}
		<StateDisplay type="loading" />
	{:else if entries.length === 0 && !error}
		<div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
			<IconHistory class="w-4 h-4" />
			{t('milestones.activity.empty')}
		</div>
	{:else}
		<ol class="space-y-3">
			{#each entries as entry (entry.id)}
				{@const parts = describeMilestoneActivity(entry, { t, formatDate: formatDateShort })}
				{@const Icon = ICONS[entry.type] ?? IconHistory}
				<li class="flex items-start gap-3" data-testid="milestone-activity-entry" data-activity-type={entry.type}>
					<div
						class="w-7 h-7 rounded-full flex items-center justify-center flex-shrink-0"
						style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
					>
						<Icon class="w-4 h-4" />
					</div>
					<div class="min-w-0 flex-1 text-sm" style="color: var(--ds-text);">
						<span class="font-medium">{parts.actor}</span>
						<span>{parts.lead}</span>
						{#if parts.target}
							<LinkComponent href={parts.target.href} class="hover:underline break-words" style="color: var(--ds-link);">
								{parts.target.label}
							</LinkComponent>
						{/if}
						{#if parts.trail}
							<span>{parts.trail}</span>
						{/if}
						<div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);" title={formatDateTimeLocale(entry.occurred_at)}>
							{formatRelativeTime(entry.occurred_at) || '-'}
						</div>
					</div>
				</li>
			{/each}
		</ol>

		{#if hasMore}
			<div class="mt-4">
				<Button size="small" onclick={loadMore} loading={loadingMore} dataTestid="milestone-activity-load-more">
					{t('common.loadMore')}
				</Button>
			</div>
		{/if}
	{/if}
</section>
