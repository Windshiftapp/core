<script>
	import { onMount } from 'svelte';
	import { api } from '../../api.js';
	import { authStore } from '../../stores';
	import { formatHistoryTimestamp, formatRelativeTime, getUserTimezone } from '../../utils/dateFormatter.js';
	import { Clock, User } from 'lucide-svelte';
	import Spinner from '../../components/Spinner.svelte';
	import AlertBox from '../../components/AlertBox.svelte';

	export let itemId;

	let history = [];
	let loading = true;
	let error = '';
	let timezone = 'UTC';

	// Get user's timezone
	$: timezone = getUserTimezone(authStore.currentUser);

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
	$: groupedHistory = groupByTimestamp(history);

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

	// Format field name for display
	function formatFieldName(fieldName) {
		// Handle special field names for attachments and diagrams
		const specialFieldNames = {
			'attachment_uploaded': 'Attachment Added',
			'attachment_deleted': 'Attachment Removed',
			'diagram_created': 'Diagram Created',
			'diagram_updated': 'Diagram Updated',
			'diagram_deleted': 'Diagram Deleted'
		};

		if (specialFieldNames[fieldName]) {
			return specialFieldNames[fieldName];
		}

		// Convert snake_case to Title Case
		return fieldName
			.split('_')
			.map(word => word.charAt(0).toUpperCase() + word.slice(1))
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
		<div class="text-center py-8" style="color: var(--ds-text-subtle);">
			<Clock class="h-12 w-12 mx-auto mb-3 opacity-50" />
			<p>No history available for this item yet.</p>
			<p class="text-sm mt-1">Changes will be tracked automatically.</p>
		</div>
	{:else}
		<div class="timeline">
			{#each groupedHistory as group}
				<div class="timeline-entry">
					<!-- User avatar and connector line -->
					<div class="timeline-avatar-container">
						<div
							class="timeline-avatar"
							style="background-color: {getUserColor(group.user_name)};"
							title={group.user_email || group.user_name}
						>
							<span class="text-white text-sm font-medium">
								{getUserInitials(group.user_name)}
							</span>
						</div>
						<div class="timeline-line"></div>
					</div>

					<!-- Change details -->
					<div class="timeline-content">
						<div class="timeline-header">
							<span class="timeline-user" style="color: var(--ds-text);">
								{group.user_name || 'Unknown User'}
							</span>
							<span class="timeline-time" style="color: var(--ds-text-subtle);" title={formatHistoryTimestamp(group.changed_at, timezone)}>
								{formatRelativeTime(group.changed_at)}
							</span>
						</div>

						<div class="timeline-changes">
							{#each group.changes as change}
								<div class="change-item">
									<span class="change-field" style="color: var(--ds-text-subtle);">
										{formatFieldName(change.field_name)}
									</span>
									<div class="change-values">
										{#if change.field_name === 'diagram_updated' && (change.old_value === null || change.old_value === undefined || change.old_value === '')}
											<!-- For diagram updates without name change, just show the diagram name -->
											<span class="change-new-value" style="color: var(--ds-text);">
												{formatValue(change.new_value, change.resolved_new_value)}
											</span>
										{:else}
											<!-- Normal display with old → new -->
											<span class="change-old-value" style="color: var(--ds-text-subtle);">
												{formatValue(change.old_value, change.resolved_old_value)}
											</span>
											<span style="color: var(--ds-text-subtle);">→</span>
											<span class="change-new-value" style="color: var(--ds-text);">
												{formatValue(change.new_value, change.resolved_new_value)}
											</span>
										{/if}
									</div>
								</div>
							{/each}
						</div>

						<!-- Full timestamp on hover -->
						<div class="timeline-full-time" style="color: var(--ds-text-subtlest);">
							{formatHistoryTimestamp(group.changed_at, timezone)}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.item-history {
		padding: 1rem;
	}

	.timeline {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.timeline-entry {
		display: flex;
		gap: 1rem;
		position: relative;
	}

	.timeline-avatar-container {
		display: flex;
		flex-direction: column;
		align-items: center;
		flex-shrink: 0;
	}

	.timeline-avatar {
		width: 40px;
		height: 40px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
	}

	.timeline-line {
		width: 2px;
		flex: 1;
		background-color: var(--ds-border);
		margin-top: 0.5rem;
		min-height: 20px;
	}

	.timeline-entry:last-child .timeline-line {
		display: none;
	}

	.timeline-content {
		flex: 1;
		min-width: 0;
		padding-bottom: 0.5rem;
	}

	.timeline-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.timeline-user {
		font-weight: 600;
		font-size: 0.9375rem;
	}

	.timeline-time {
		font-size: 0.875rem;
	}

	.timeline-changes {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.75rem;
		background-color: var(--ds-surface-raised);
		border: 1px solid var(--ds-border);
		border-radius: 6px;
	}

	.change-item {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.change-field {
		font-weight: 500;
		font-size: 0.8125rem;
		text-transform: capitalize;
	}

	.change-values {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		flex-wrap: wrap;
	}

	.change-old-value {
		text-decoration: line-through;
		opacity: 0.7;
	}

	.change-new-value {
		font-weight: 500;
	}

	.timeline-full-time {
		margin-top: 0.5rem;
		font-size: 0.75rem;
	}

	/* Responsive adjustments */
	@media (max-width: 640px) {
		.timeline-avatar {
			width: 32px;
			height: 32px;
		}

		.timeline-avatar span {
			font-size: 0.75rem;
		}

		.timeline-header {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.25rem;
		}
	}
</style>
