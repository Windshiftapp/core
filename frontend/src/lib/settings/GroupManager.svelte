<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { Plus, Edit, Trash2, UserStar, UserPlus, UserMinus, Circle, CheckCircle, X } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import Input from '../components/Input.svelte';
	import Textarea from '../components/Textarea.svelte';
	import DataTable from '../components/DataTable.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import UserPicker from '../pickers/UserPicker.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { t } from '../stores/i18n.svelte.js';
	import { confirm } from '../composables/useConfirm.js';
	import { formatDateSimple } from '../utils/dateFormatter.js';

	let groups = $state([]);
	let users = $state([]);
	let loading = $state(false);
	let error = $state('');
	let showCreateForm = $state(false);
	let editingGroup = $state(null);
	let showMemberModal = $state(false);
	let selectedGroup = $state(null);
	let availableUsers = $state([]);
	let selectedUserIds = $state([]);
	let selectedUserId = $state(null);
	let selectedUsersToAdd = $state([]);

	// Form data
	let formData = $state({
		name: '',
		description: ''
	});

	async function loadGroups() {
		loading = true;
		try {
			groups = await api.groups.getAll();
			error = '';
		} catch (err) {
			error = err.message || t('settings.groups.failedToLoad');
		} finally {
			loading = false;
		}
	}

	async function loadUsers() {
		try {
			users = await api.getUsers();
		} catch (err) {
			console.error('Failed to load users:', err);
		}
	}

	async function saveGroup() {
		try {
			if (editingGroup) {
				await api.groups.update(editingGroup.id, {
					...formData,
					is_active: true // Always keep groups active when editing
				});
			} else {
				await api.groups.create(formData);
			}
			
			resetForm();
			await loadGroups();
		} catch (err) {
			error = err.message || t('settings.groups.failedToSave');
		}
	}

	async function deleteGroup(groupId) {
		const confirmed = await confirm({
			title: t('common.delete'),
			message: t('settings.groups.confirmDelete'),
			confirmText: t('common.delete'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!confirmed) return;

		try {
			await api.groups.delete(groupId);
			await loadGroups();
		} catch (err) {
			error = err.message || t('settings.groups.failedToDelete');
		}
	}

	async function manageMembers(group) {
		selectedGroup = group;
		
		// Get detailed group info with current members
		try {
			const groupDetail = await api.groups.get(group.id);
			selectedGroup = groupDetail;
			selectedUsersToAdd = [];
			selectedUserId = null;
			
			showMemberModal = true;
		} catch (err) {
			error = err.message || t('settings.groups.failedToLoadDetails');
		}
	}

	function onUserSelected(user) {
		if (!user || user.id === null) return;
		
		// Check if user is already in the "to add" list or already a member
		const isAlreadyToAdd = selectedUsersToAdd.some(u => u.id === user.id);
		const isAlreadyMember = selectedGroup.members && selectedGroup.members.some(m => m.user_id === user.id);
		
		if (!isAlreadyToAdd && !isAlreadyMember) {
			selectedUsersToAdd = [...selectedUsersToAdd, user];
		}
		
		// Clear the picker
		selectedUserId = null;
	}

	function removeUserFromAddList(userId) {
		selectedUsersToAdd = selectedUsersToAdd.filter(u => u.id !== userId);
	}

	async function addSelectedMembers() {
		if (selectedUsersToAdd.length === 0) return;
		
		try {
			const userIds = selectedUsersToAdd.map(u => u.id);
			await api.groups.addMembers(selectedGroup.id, userIds);
			
			// Refresh group details
			const groupDetail = await api.groups.get(selectedGroup.id);
			selectedGroup = groupDetail;
			
			// Clear the selection
			selectedUsersToAdd = [];
			selectedUserId = null;
			
			// Refresh groups list
			await loadGroups();
		} catch (err) {
			error = err.message || t('settings.groups.failedToAddMembers');
		}
	}

	async function removeMember(userId) {
		const confirmed = await confirm({
			title: t('common.remove'),
			message: t('settings.groups.confirmRemoveMember'),
			confirmText: t('common.remove'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!confirmed) return;
		
		try {
			await api.groups.removeMembers(selectedGroup.id, [userId]);
			
			// Refresh group details
			const groupDetail = await api.groups.get(selectedGroup.id);
			selectedGroup = groupDetail;
			
			// Refresh groups list
			await loadGroups();
		} catch (err) {
			error = err.message || t('settings.groups.failedToRemoveMember');
		}
	}

	function closeMemberModal() {
		showMemberModal = false;
		selectedGroup = null;
		availableUsers = [];
		selectedUserIds = [];
		selectedUserId = null;
		selectedUsersToAdd = [];
	}

	function buildGroupDropdownItems(group) {
		const items = [
			{
				id: 'edit',
				type: 'regular',
				icon: Edit,
				title: t('settings.groups.edit'),
				hoverClass: 'hover-bg',
				onClick: () => editGroup(group)
			},
			{
				id: 'members',
				type: 'regular',
				icon: UserStar,
				title: t('settings.groups.manageMembers'),
				hoverClass: 'hover-bg',
				onClick: () => manageMembers(group)
			}
		];

		// Only add delete for non-system groups
		if (!group.is_system_group) {
			items.push({
				id: 'delete',
				type: 'regular',
				icon: Trash2,
				title: t('settings.groups.delete'),
				color: '#dc2626',
				hoverClass: 'hover:bg-red-50',
				onClick: () => deleteGroup(group.id)
			});
		}

		return items;
	}

	// Table column definitions (reactive for i18n)
	const groupColumns = $derived([
		{
			key: 'name',
			label: t('settings.groups.groupName')
		},
		{
			key: 'description',
			label: t('settings.groups.description'),
			textColor: 'var(--ds-text-subtle)'
		},
		{
			key: 'member_count',
			label: t('settings.groups.members'),
			textColor: 'var(--ds-text-subtle)',
			render: (group) => {
				const count = group.member_count || 0;
				return count === 0 ? t('settings.groups.noMembers') : t('settings.groups.membersCount', { count });
			}
		},
		{
			key: 'ldap_sync_enabled',
			label: t('settings.groups.ldapSync'),
			slot: 'ldap_sync'
		},
		{
			key: 'is_active',
			label: t('settings.groups.status'),
			slot: 'status'
		},
		{
			key: 'actions',
			label: t('settings.groups.actions')
		}
	]);

	function resetForm() {
		formData = {
			name: '',
			description: ''
		};
		editingGroup = null;
		showCreateForm = false;
	}

	function editGroup(group) {
		formData = {
			name: group.name,
			description: group.description || ''
		};
		editingGroup = group;
		showCreateForm = true;
	}

	onMount(() => {
		loadGroups();
		loadUsers();
	});
</script>

<div class="space-y-6">
	<PageHeader
		icon={UserStar}
		title={t('settings.groups.title')}
		subtitle={t('settings.groups.subtitle')}
	>
		{#snippet actions()}
			<Button
				variant="primary"
				icon={Plus}
				onclick={() => {
					resetForm();
					showCreateForm = true;
				}}
				keyboardHint="A"
				hotkeyConfig={{ key: toHotkeyString('groups', 'add'), guard: () => !showCreateForm }}
			>
				{t('settings.groups.addGroup')}
			</Button>
		{/snippet}
	</PageHeader>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<Modal isOpen={showCreateForm} onclose={resetForm} maxWidth="max-w-lg">
		<ModalHeader
			title={editingGroup ? t('settings.groups.editGroup') : t('settings.groups.createGroup')}
			onClose={resetForm}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			<form onsubmit={(e) => { e.preventDefault(); saveGroup(); }} class="space-y-4">
				<div>
					<label for="name" class="block text-sm font-medium" style="color: var(--ds-text)">
						{t('settings.groups.groupName')}
					</label>
					<Input
						id="name"
						bind:value={formData.name}
						required
						placeholder={t('settings.groups.groupNamePlaceholder')}
					/>
				</div>

				<div>
					<label for="description" class="block text-sm font-medium" style="color: var(--ds-text)">
						{t('settings.groups.descriptionOptional')}
					</label>
					<Textarea
						id="description"
						bind:value={formData.description}
						rows={3}
						placeholder={t('settings.groups.descriptionPlaceholder')}
					/>
				</div>
			</form>
		</div>

		<DialogFooter
			confirmLabel={editingGroup ? t('settings.groups.updateGroup') : t('settings.groups.createGroup')}
			onCancel={resetForm}
			onConfirm={saveGroup}
		/>
	</Modal>

	<Modal isOpen={showMemberModal && selectedGroup} onclose={closeMemberModal} maxWidth="max-w-2xl">
		<ModalHeader
			title="{t('settings.groups.manageMembers')}: {selectedGroup?.name}"
			subtitle={t('settings.groups.membersCount', { count: selectedGroup?.member_count || 0 })}
			onClose={closeMemberModal}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4 space-y-6 max-h-[60vh] overflow-y-auto">
					<!-- Current Members -->
					{#if selectedGroup.members && selectedGroup.members.length > 0}
						<div>
							<h4 class="text-sm font-medium mb-3" style="color: var(--ds-text)">{t('settings.groups.currentMembers')}</h4>
							<div class="space-y-2">
								{#each selectedGroup.members as member}
									<div class="flex items-center justify-between p-3 rounded border" style="background-color: var(--ds-surface); border-color: var(--ds-border)">
										<div class="flex items-center">
											<div class="h-8 w-8 rounded-full flex items-center justify-center mr-3" style="background-color: var(--ds-background-neutral)">
												<span class="text-xs font-medium" style="color: var(--ds-text)">
													{member.user_name ? member.user_name.split(' ').map(n => n.charAt(0)).join('') : '?'}
												</span>
											</div>
											<div>
												<div class="text-sm font-medium" style="color: var(--ds-text)">
													{member.user_name || t('settings.groups.unknownUser')}
												</div>
												<div class="text-xs" style="color: var(--ds-text-subtle)">
													{member.user_email} • {t('settings.groups.added')} {formatDateSimple(member.added_at)}
													{#if member.ldap_sync_enabled}
														<span class="ml-2 inline-flex px-2 py-0.5 text-xs font-medium bg-blue-100 text-blue-800 rounded">
															LDAP
														</span>
													{/if}
												</div>
											</div>
										</div>
										{#if !member.ldap_sync_enabled}
											<Button
												variant="ghost"
												size="sm"
												icon={UserMinus}
												onclick={() => removeMember(member.user_id)}
											>
												{t('settings.groups.remove')}
											</Button>
										{:else}
											<span class="text-xs" style="color: var(--ds-text-subtlest)">{t('settings.groups.ldapManaged')}</span>
										{/if}
									</div>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Add Members -->
					<div>
						<h4 class="text-sm font-medium mb-3" style="color: var(--ds-text)">{t('settings.groups.addMembers')}</h4>
						<div class="space-y-4">
							<!-- User Picker -->
							<div>
								<UserPicker
									bind:value={selectedUserId}
									placeholder={t('settings.groups.searchAndSelectUser')}
									onSelect={onUserSelected}
								/>
							</div>

							<!-- Selected Users to Add -->
							{#if selectedUsersToAdd.length > 0}
								<div>
									<p class="text-sm mb-2" style="color: var(--ds-text-subtle)">{t('settings.groups.usersToAdd')} ({selectedUsersToAdd.length}):</p>
									<div class="space-y-2 max-h-32 overflow-y-auto">
										{#each selectedUsersToAdd as user}
											<div class="flex items-center justify-between p-2 bg-blue-50 rounded border dark:bg-blue-900/20">
												<div class="flex items-center">
													<div class="h-6 w-6 rounded-full bg-blue-500 flex items-center justify-center mr-2">
														<span class="text-xs font-medium text-white">
															{user.first_name.charAt(0)}{user.last_name.charAt(0)}
														</span>
													</div>
													<div>
														<div class="text-sm font-medium" style="color: var(--ds-text)">
															{user.first_name} {user.last_name}
														</div>
														<div class="text-xs" style="color: var(--ds-text-subtle)">
															{user.email}
														</div>
													</div>
												</div>
												<button
													class="p-1 rounded"
													style="color: var(--ds-text-subtle)"
													onclick={() => removeUserFromAddList(user.id)}
													title={t('settings.groups.remove')}
												>
													<X class="w-4 h-4" />
												</button>
											</div>
										{/each}
									</div>

									<div class="flex justify-end mt-3">
										<Button
											variant="primary"
											size="sm"
											icon={UserPlus}
											onclick={addSelectedMembers}
										>
											{t('common.add')} {selectedUsersToAdd.length === 1 ? t('settings.groups.memberCount', { count: selectedUsersToAdd.length }) : t('settings.groups.membersCount', { count: selectedUsersToAdd.length })}
										</Button>
									</div>
								</div>
							{/if}
						</div>
					</div>
				</div>

		<DialogFooter
			showCancel={false}
			confirmLabel={t('common.close')}
			variant="secondary"
			onConfirm={closeMemberModal}
		/>
	</Modal>

	{#if loading}
		<div class="text-center py-8">
			<div style="color: var(--ds-text-subtle)">{t('settings.groups.loadingGroups')}</div>
		</div>
	{:else}
		<DataTable
			columns={groupColumns}
			data={groups}
			keyField="id"
			emptyMessage={t('settings.groups.noGroupsFound')}
			emptyIcon={Circle}
			actionItems={buildGroupDropdownItems}
		>
			{#snippet ldap_sync(group)}
				<Lozenge color={group.ldap_sync_enabled ? 'blue' : 'gray'} text={group.ldap_sync_enabled ? t('settings.groups.enabled') : t('settings.groups.manual')} />
			{/snippet}

			{#snippet status(group)}
				<Lozenge color={group.is_active ? 'green' : 'red'} text={group.is_active ? t('settings.groups.active') : t('settings.groups.inactive')} />
			{/snippet}
		</DataTable>
	{/if}
</div>
