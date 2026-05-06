<script>
	import { onMount } from 'svelte';
	import { api } from '../../api.js';
	import { authStore } from '../../stores';
	import { formatHistoryTimestamp, formatRelativeTime, getUserTimezone } from '../../utils/dateFormatter.js';
	import { Clock, User } from 'lucide-svelte';
	import Spinner from '../../components/Spinner.svelte';
	import AlertBox from '../../components/AlertBox.svelte';
	import EmptyState from '../../components/EmptyState.svelte';

	let { itemId } = $props();

	let history = $state([]);
	let loading = $state(true);
	let error = $state('');

	// Get user's timezone
	let timezone = $derived(getUserTimezone(authStore.currentUser));

	onMount(() => {
		loadHistory();
	});

	async function loadHistory() {
		loading = true;
		error = '';
		try {
			history = await api.items.getHistory(itemId);
		} catch (err) {
			error = err.message || 'Failed to load item history';
			console.error('Error loading item history:', err);
		} finally {
			loading = false;
		}
	}

	// Group history entries by timestamp (changes made at the same time)
	let groupedHistory = $derived(groupByTimestamp(history));

	function groupByTimestamp(entries) {
		if (!entries || entries.length === 0) return [];

		const groups = [];
		let currentGroup = null;

		entries.forEach(entry => {
			const timestamp = new Date(entry.changed_at).getTime();

			// If no current group or timestamp differs by more than 1 second, start new group
			if (!currentGroup || Math.abs(currentGroup.timestamp - timestamp) > 1000) {
				currentGroup = {
					timestamp,
					changed_at: entry.changed_at,
					user_id: entry.user_id,
					user_name: entry.user_name,
					user_email: entry.user_email,
					changes: []
				};
				groups.push(currentGroup);
			}

			currentGroup.changes.push({
				field_name: entry.field_name,
				old_value: entry.old_value,
				new_value: entry.new_value,
				resolved_old_value: entry.resolved_old_value,
				resolved_new_value: entry.resolved_new_value
			});
		});

		return groups;
	}

	// Approval-engine events are merged into the history feed server-side as
	// rows with field_name="approval_<decision>" and new_value=comment. They
	// render as a single line (no "old → new"), since there's no prior value.
	function isApprovalEntry(fieldName) {
		return typeof fieldName === 'string' && fieldName.startsWith('approval_');
	}

	// Format field name for display
	function formatFieldName(fieldName) {
		// Handle special field names for attachments and diagrams
		const specialFieldNames = {
			'attachment_uploaded': 'attachment',
			'attachment_deleted': 'attachment removed',
			'diagram_created': 'diagram',
			'diagram_updated': 'diagram',
			'diagram_deleted': 'diagram removed',
			'approval_requested': 'Approval requested',
			'approval_approve': 'Approved',
			'approval_reject': 'Rejected',
			'approval_comment': 'commented on approval',
			'approval_cancel': 'cancelled approval',
			'approval_delegate': 'delegated approval',
			'approval_reassign': 'reassigned approvers',
			'approval_escalate': 'escalated approval',
			'approval_substitute': 'used substitute approver',
			'approval_completed': 'Approval completed'
		};

		if (specialFieldNames[fieldName]) {
			return specialFieldNames[fieldName];
		}

		// Strip a trailing _id (e.g. "status_id" → "status") then humanize.
		const cleaned = fieldName.replace(/_id$/, '');
		return cleaned
			.split('_')
			.map(word => word.charAt(0).toLowerCase() + word.slice(1))
			.join(' ');
	}

	// Format field value for display
	function formatValue(value, resolvedValue) {
		// If we have a resolved value (human-readable), use that instead
		if (resolvedValue && resolvedValue !== '') {
			return resolvedValue;
		}

		if (value === null || value === undefined || value === '') {
			return 'None';
		}

		// Parse attachment and diagram values (format: "attachment:id:filename" or "diagram:id:name")
		if (typeof value === 'string') {
			if (value.startsWith('attachment:')) {
				const parts = value.split(':');
				if (parts.length >= 3) {
					return parts.slice(2).join(':'); // Return filename (in case filename contains ':')
				}
			} else if (value.startsWith('diagram:')) {
				const parts = value.split(':');
				if (parts.length >= 3) {
					return parts.slice(2).join(':'); // Return diagram name (in case name contains ':')
				}
			}
		}

		// Try to parse as JSON for custom fields
		if (typeof value === 'string' && (value.startsWith('{') || value.startsWith('['))) {
			try {
				const parsed = JSON.parse(value);
				return JSON.stringify(parsed, null, 2);
			} catch (e) {
				// Not valid JSON, return as-is
			}
		}

		// Truncate long values
		if (typeof value === 'string' && value.length > 100) {
			return value.substring(0, 100) + '...';
		}

		return value;
	}

	// Get a color for the user avatar
	function getUserColor(userName) {
		if (!userName) return '#6B7280';
		const colors = ['#EF4444', '#F59E0B', '#10B981', '#3B82F6', '#6366F1', '#8B5CF6', '#EC4899'];
		const hash = userName.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0);
		return colors[hash % colors.length];
	}

	// Get user initials
	function getUserInitials(userName) {
		if (!userName) return '?';
		const parts = userName.trim().split(' ');
		if (parts.length >= 2) {
			return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
		}
		return userName.substring(0, 2).toUpperCase();
	}
</script>

<div class="item-history">
	{#if loading}
		<div class="flex items-center justify-center py-8">
			<Spinner />
		</div>
	{:else if error}
		<AlertBox message={error} />
	{:else if groupedHistory.length === 0}
		<EmptyState
			icon={Clock}
			title="No history available for this item yet."
			description="Changes will be tracked automatically."
		/>
	{:else}
		<ul class="timeline">
			{#each groupedHistory as group}
				<li class="entry">
					<div class="rail">
						<div
							class="avatar"
							style="background-color: {getUserColor(group.user_name)};"
							title={group.user_email || group.user_name}
						>
							{getUserInitials(group.user_name)}
						</div>
						<div class="line"></div>
					</div>
					<div class="body">
						<div class="header">
							<span class="user">{group.user_name || 'Unknown'}</span>
							<span class="time" title={formatHistoryTimestamp(group.changed_at, timezone)}>
								{formatRelativeTime(group.changed_at)}
							</span>
						</div>
						{#each group.changes as change}
							<div class="change">
								{#if isApprovalEntry(change.field_name)}
									<span class="action">{formatFieldName(change.field_name)}</span>
									{#if change.new_value && change.new_value !== ''}
										<span class="quote">"{formatValue(change.new_value, change.resolved_new_value)}"</span>
									{/if}
								{:else if change.field_name === 'diagram_updated' && (change.old_value === null || change.old_value === undefined || change.old_value === '')}
									<span class="action">updated</span>
									<span class="new">{formatValue(change.new_value, change.resolved_new_value)}</span>
								{:else}
									<span class="action">changed {formatFieldName(change.field_name)}</span>
									<span class="old">{formatValue(change.old_value, change.resolved_old_value)}</span>
									<span class="arrow">→</span>
									<span class="new">{formatValue(change.new_value, change.resolved_new_value)}</span>
								{/if}
							</div>
						{/each}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.item-history {
		padding: 0.75rem 1rem;
	}

	.timeline {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.entry {
		display: flex;
		gap: 0.75rem;
	}

	.rail {
		display: flex;
		flex-direction: column;
		align-items: center;
		flex-shrink: 0;
		width: 2rem;
	}

	.avatar {
		width: 2rem;
		height: 2rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.75rem;
		font-weight: 600;
		color: white;
		flex-shrink: 0;
	}

	.line {
		width: 1px;
		flex: 1;
		background-color: var(--ds-border);
		margin-top: 0.25rem;
		min-height: 0.5rem;
	}

	.entry:last-child .line {
		display: none;
	}

	.body {
		flex: 1;
		min-width: 0;
		padding-bottom: 0.875rem;
	}

	.header {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		line-height: 2rem;
	}

	.user {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--ds-text);
	}

	.time {
		font-size: 0.75rem;
		color: var(--ds-text-subtlest);
		white-space: nowrap;
	}

	.change {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.375rem;
		font-size: 0.8125rem;
		line-height: 1.375rem;
		color: var(--ds-text-subtle);
	}

	.action {
		color: var(--ds-text-subtle);
	}

	.old {
		text-decoration: line-through;
		opacity: 0.7;
	}

	.new {
		font-weight: 500;
		color: var(--ds-text);
	}

	.arrow {
		color: var(--ds-text-subtlest);
	}

	.quote {
		font-style: italic;
		color: var(--ds-text);
	}
</style>
