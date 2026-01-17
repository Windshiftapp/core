<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { FileText, Download, Filter, Eye } from 'lucide-svelte';
	import SearchInput from '../components/SearchInput.svelte';
	import Button from '../components/Button.svelte';
	import Input from '../components/Input.svelte';
	import Select from '../components/Select.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import DataTable from '../components/DataTable.svelte';
	import Pagination from '../components/Pagination.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import Label from '../components/Label.svelte';
	import { formatDate, formatDateTimeLocale } from '../utils/dateFormatter.js';
	import { t } from '../stores/i18n.svelte.js';

	let logs = $state([]);
	let pagination = $state({
		page: 1,
		limit: 50,
		total: 0,
		totalPages: 0
	});
	let loading = $state(false);
	let error = $state('');

	// Filters
	let filters = $state({
		user_id: '',
		action_type: '',
		resource_type: '',
		success: '',
		start_date: '',
		end_date: '',
		search: ''
	});

	// Detail modal
	let showDetailModal = $state(false);
	let selectedLog = $state(null);

	// Common action types for filter
	const actionTypes = [
		{ value: '', label: t('auditLog.allActions') },
		{ value: 'user.create', label: t('auditLog.userCreated') },
		{ value: 'user.update', label: t('auditLog.userUpdated') },
		{ value: 'user.delete', label: t('auditLog.userDeleted') },
		{ value: 'user.activate', label: t('auditLog.userActivated') },
		{ value: 'user.deactivate', label: t('auditLog.userDeactivated') },
		{ value: 'user.password_reset', label: t('auditLog.passwordReset') },
		{ value: 'api_token.create', label: t('auditLog.apiTokenCreated') },
		{ value: 'api_token.revoke', label: t('auditLog.apiTokenRevoked') },
		{ value: 'workspace.create', label: t('auditLog.workspaceCreated') },
		{ value: 'workspace.update', label: t('auditLog.workspaceUpdated') },
		{ value: 'workspace.delete', label: t('auditLog.workspaceDeleted') },
		{ value: 'group.create', label: t('auditLog.groupCreated') },
		{ value: 'group.update', label: t('auditLog.groupUpdated') },
		{ value: 'group.delete', label: t('auditLog.groupDeleted') },
		{ value: 'group.add_member', label: t('auditLog.groupMemberAdded') },
		{ value: 'group.remove_member', label: t('auditLog.groupMemberRemoved') },
		{ value: 'custom_field.create', label: t('auditLog.customFieldCreated') },
		{ value: 'custom_field.update', label: t('auditLog.customFieldUpdated') },
		{ value: 'custom_field.delete', label: t('auditLog.customFieldDeleted') },
		{ value: 'item_type.create', label: t('auditLog.itemTypeCreated') },
		{ value: 'item_type.update', label: t('auditLog.itemTypeUpdated') },
		{ value: 'item_type.delete', label: t('auditLog.itemTypeDeleted') },
		{ value: 'permission.grant', label: t('auditLog.permissionGranted') },
		{ value: 'permission.revoke', label: t('auditLog.permissionRevoked') },
		{ value: 'role.assign', label: t('auditLog.roleAssigned') },
		{ value: 'role.revoke', label: t('auditLog.roleRevoked') },
	];

	// Common resource types
	const resourceTypes = [
		{ value: '', label: t('auditLog.allResources') },
		{ value: 'user', label: t('auditLog.user') },
		{ value: 'api_token', label: t('auditLog.apiToken') },
		{ value: 'workspace', label: t('auditLog.workspace') },
		{ value: 'custom_field', label: t('auditLog.customField') },
		{ value: 'item_type', label: t('auditLog.itemType') },
		{ value: 'permission', label: t('auditLog.permission') },
		{ value: 'role', label: t('auditLog.role') },
		{ value: 'group', label: t('auditLog.group') },
	];

	async function loadAuditLogs() {
		loading = true;
		error = '';
		try {
			const activeFilters = { ...filters, page: pagination.page, limit: pagination.limit };
			const response = await api.auditLogs.getAll(activeFilters);
			logs = response.logs || [];
			pagination = response.pagination;
		} catch (err) {
			error = err.message || 'Failed to load audit logs';
			logs = [];
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		pagination.page = 1; // Reset to first page when filtering
		loadAuditLogs();
	}

	function clearFilters() {
		filters = {
			user_id: '',
			action_type: '',
			resource_type: '',
			success: '',
			start_date: '',
			end_date: '',
			search: ''
		};
		applyFilters();
	}

	function handlePageChange(event) {
		pagination.page = event.detail.page;
		pagination.limit = event.detail.itemsPerPage;
		loadAuditLogs();
	}

	function handlePageSizeChange(event) {
		pagination.page = event.detail.page;
		pagination.limit = event.detail.itemsPerPage;
		loadAuditLogs();
	}

	function exportLogs(format) {
		const activeFilters = { ...filters };
		const exportUrl = api.auditLogs.export(format, activeFilters);
		// Trigger download
		window.location.href = exportUrl;
	}

	function viewDetails(log) {
		selectedLog = log;
		showDetailModal = true;
	}

	function getActionBadgeColor(actionType) {
		if (actionType.includes('create')) return 'green';
		if (actionType.includes('update')) return 'blue';
		if (actionType.includes('delete')) return 'red';
		if (actionType.includes('grant') || actionType.includes('assign')) return 'purple';
		if (actionType.includes('revoke')) return 'orange';
		if (actionType.includes('activate')) return 'green';
		if (actionType.includes('deactivate')) return 'orange';
		return 'gray';
	}

	// Table column definitions
	const auditColumns = [
		{
			key: 'timestamp',
			label: t('auditLog.timestamp'),
			render: (log) => formatDateTimeLocale(log.timestamp) || '-'
		},
		{
			key: 'username',
			label: t('auditLog.user'),
			slot: 'user'
		},
		{
			key: 'action_type',
			label: t('auditLog.action'),
			slot: 'action'
		},
		{
			key: 'resource_name',
			label: t('auditLog.resource'),
			slot: 'resource'
		},
		{
			key: 'ip_address',
			label: t('auditLog.ipAddress'),
			render: (log) => log.ip_address || '—',
			textColor: 'var(--ds-text-subtle)'
		},
		{
			key: 'success',
			label: t('auditLog.status'),
			slot: 'status'
		},
		{
			key: 'details',
			label: t('auditLog.details'),
			slot: 'details',
			width: 'w-24'
		}
	];

	onMount(() => {
		loadAuditLogs();
	});
</script>

<div class="space-y-6">
	<PageHeader
		icon={FileText}
		title={t('audit.title')}
		subtitle={t('audit.subtitle')}
	>
		{#snippet actions()}
			<div class="flex gap-2">
				<Button
					variant="secondary"
					icon={Download}
					onclick={() => exportLogs('csv')}
				>
					{t('auditLog.exportCsv')}
				</Button>
				<Button
					variant="secondary"
					icon={Download}
					onclick={() => exportLogs('json')}
				>
					{t('auditLog.exportJson')}
				</Button>
			</div>
		{/snippet}
	</PageHeader>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<!-- Filters -->
	<div class="bg-white rounded shadow p-4" style="border: 1px solid var(--ds-border);">
		<div class="flex items-center gap-2 mb-4">
			<Filter class="w-5 h-5" style="color: var(--ds-text-subtle);" />
			<h3 class="text-sm font-medium" style="color: var(--ds-text);">{t('auditLog.filters')}</h3>
		</div>

		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
			<div>
				<Label for="action_type" class="mb-1">{t('auditLog.actionType')}</Label>
				<Select id="action_type" bind:value={filters.action_type} size="small">
					{#each actionTypes as actionType}
						<option value={actionType.value}>{actionType.label}</option>
					{/each}
				</Select>
			</div>

			<div>
				<Label for="resource_type" class="mb-1">{t('auditLog.resourceType')}</Label>
				<Select id="resource_type" bind:value={filters.resource_type} size="small">
					{#each resourceTypes as resourceType}
						<option value={resourceType.value}>{resourceType.label}</option>
					{/each}
				</Select>
			</div>

			<div>
				<Label for="success" class="mb-1">{t('auditLog.status')}</Label>
				<Select id="success" bind:value={filters.success} size="small">
					<option value="">{t('auditLog.all')}</option>
					<option value="true">{t('auditLog.success')}</option>
					<option value="false">{t('auditLog.failed')}</option>
				</Select>
			</div>

			<div>
				<Label for="search" class="mb-1">{t('auditLog.search')}</Label>
				<SearchInput
					bind:value={filters.search}
					placeholder="Username, resource..."
					on_keydown={(e) => e.key === 'Enter' && applyFilters()}
				/>
			</div>

			<div>
				<Label for="start_date" class="mb-1">{t('auditLog.startDate')}</Label>
				<Input id="start_date" type="date" bind:value={filters.start_date} size="small" />
			</div>

			<div>
				<Label for="end_date" class="mb-1">{t('auditLog.endDate')}</Label>
				<Input id="end_date" type="date" bind:value={filters.end_date} size="small" />
			</div>
		</div>

		<div class="flex gap-2 mt-4">
			<Button variant="primary" onclick={applyFilters}>
				{t('auditLog.applyFilters')}
			</Button>
			<Button variant="secondary" onclick={clearFilters}>
				{t('auditLog.clearFilters')}
			</Button>
		</div>
	</div>

	<!-- Audit Log Table -->
	{#if loading}
		<div class="text-center py-8">
			<div style="color: var(--ds-text-subtle);">{t('auditLog.loadingAuditLogs')}</div>
		</div>
	{:else}
		<div class="space-y-0">
			<DataTable
				columns={auditColumns}
				data={logs}
				keyField="id"
				emptyMessage={t('auditLog.noAuditLogs')}
				emptyIcon={FileText}
			>
				<div slot="user" let:item={log}>
					<div class="font-medium" style="color: var(--ds-text);">{log.username}</div>
					{#if log.user_id}
						<div class="text-xs" style="color: var(--ds-text-subtle);">ID: {log.user_id}</div>
					{/if}
				</div>

				<Lozenge slot="action" let:item={log} color={getActionBadgeColor(log.action_type)} text={log.action_type} />

				<div slot="resource" let:item={log}>
					<div class="font-medium" style="color: var(--ds-text);">{log.resource_name || '—'}</div>
					<div class="text-xs" style="color: var(--ds-text-subtle);">{log.resource_type}</div>
				</div>

				<Lozenge slot="status" let:item={log} color={log.success ? 'green' : 'red'} text={log.success ? t('auditLog.success') : t('auditLog.failed')} />

				<Button slot="details" let:item={log} variant="ghost" icon={Eye} size="small" onclick={() => viewDetails(log)} title={t('common.viewDetails')} />
			</DataTable>

			<!-- Pagination -->
			{#if pagination.total > 0}
				<div class="mt-6">
					<Pagination
						currentPage={pagination.page}
						totalItems={pagination.total}
						itemsPerPage={pagination.limit}
						maxItems={10000}
						pageSizeOptions={[25, 50, 100]}
						onpageChange={handlePageChange}
						onpageSizeChange={handlePageSizeChange}
					/>
				</div>
			{/if}
		</div>
	{/if}
</div>

<!-- Detail Modal -->
<Modal isOpen={showDetailModal} onclose={() => showDetailModal = false} maxWidth="max-w-3xl">
	{#if selectedLog}
		<ModalHeader
			title={t('auditLog.auditLogDetails')}
			onClose={() => showDetailModal = false}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4 space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<Label class="mb-1">{t('auditLog.timestamp')}</Label>
					<div class="text-sm" style="color: var(--ds-text);">{formatDateTimeLocale(selectedLog.timestamp) || '-'}</div>
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.user')}</Label>
					<div class="text-sm" style="color: var(--ds-text);">{selectedLog.username} (ID: {selectedLog.user_id || 'N/A'})</div>
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.action')}</Label>
					<Lozenge color={getActionBadgeColor(selectedLog.action_type)} text={selectedLog.action_type} />
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.status')}</Label>
					<Lozenge color={selectedLog.success ? 'green' : 'red'} text={selectedLog.success ? t('auditLog.success') : t('auditLog.failed')} />
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.resourceType')}</Label>
					<div class="text-sm" style="color: var(--ds-text);">{selectedLog.resource_type}</div>
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.resourceName')}</Label>
					<div class="text-sm" style="color: var(--ds-text);">{selectedLog.resource_name || 'N/A'}</div>
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.ipAddress')}</Label>
					<div class="text-sm" style="color: var(--ds-text);">{selectedLog.ip_address || 'N/A'}</div>
				</div>
				<div>
					<Label class="mb-1">{t('auditLog.userAgent')}</Label>
					<div class="text-sm truncate" style="color: var(--ds-text);" title={selectedLog.user_agent}>
						{selectedLog.user_agent || 'N/A'}
					</div>
				</div>
			</div>

			{#if !selectedLog.success && selectedLog.error_message}
				<div>
					<label class="block text-sm font-medium mb-1 text-red-700">{t('auditLog.errorMessage')}</label>
					<div class="text-sm bg-red-50 p-3 rounded border border-red-200 text-red-700">
						{selectedLog.error_message}
					</div>
				</div>
			{/if}

			{#if selectedLog.details && Object.keys(selectedLog.details).length > 0}
				<div>
					<Label class="mb-2">{t('auditLog.additionalDetails')}</Label>
					<div class="bg-gray-50 p-4 rounded border" style="border-color: var(--ds-border);">
						<pre class="text-xs overflow-auto" style="color: var(--ds-text);">{JSON.stringify(selectedLog.details, null, 2)}</pre>
					</div>
				</div>
			{/if}
		</div>

		<DialogFooter
			showCancel={false}
			confirmLabel={t('auditLog.close')}
			onConfirm={() => showDetailModal = false}
		/>
	{/if}
</Modal>
