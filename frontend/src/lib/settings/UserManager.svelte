<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { Plus, Edit, Trash2, RotateCcw, MoreHorizontal, Circle, CheckCircle, Copy, Key, Users, UserCheck, UserX, AlertTriangle } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import Input from '../components/Input.svelte';
	import DataTable from '../components/DataTable.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import SearchInput from '../components/SearchInput.svelte';
	import { errorToast } from '../stores/toasts.svelte.js';
	import AlertBox from '../components/AlertBox.svelte';
	import Lozenge from '../components/Lozenge.svelte';
	import Label from '../components/Label.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { t } from '../stores/i18n.svelte.js';

	let users = $state([]);
	let loading = $state(false);
	let error = $state('');
	let searchQuery = $state('');
	let showCreateForm = $state(false);
	let editingUser = $state(null);
	let showPasswordResetModal = $state(false);
	let resetPasswordUser = $state(null);
	let newPassword = $state('');
	let generateRandomPassword = $state(true);
	let showPasswordResultModal = $state(false);
	let temporaryPassword = $state('');
	let passwordResetSuccess = $state(false);
	let resetPasswordUserName = $state('');


	// Confirmation dialog state
	let showConfirmDialog = $state(false);
	let confirmAction = $state(null);
	let confirmTitle = $state('');
	let confirmMessage = $state('');
	let confirmButtonText = $state('');
	let confirmButtonVariant = $state('danger');

	// Form data
	let formData = $state({
		email: '',
		username: '',
		first_name: '',
		last_name: '',
		password: '',
		is_active: true
	});

	async function loadUsers() {
		loading = true;
		try {
			users = await api.getUsers();
			error = '';
		} catch (err) {
			error = err.message || t('users.failedToLoad');
		} finally {
			loading = false;
		}
	}

	async function saveUser() {
		try {
			if (editingUser) {
				await api.updateUser(editingUser.id, formData);
			} else {
				await api.createUser(formData);
			}

			resetForm();
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToSave');
			error = errorMsg;
			errorToast(errorMsg);
		}
	}

	function showConfirm(title, message, buttonText, action, variant = 'danger') {
		confirmTitle = title;
		confirmMessage = message;
		confirmButtonText = buttonText;
		confirmAction = action;
		confirmButtonVariant = variant;
		showConfirmDialog = true;
	}

	function closeConfirmDialog() {
		showConfirmDialog = false;
		confirmAction = null;
		confirmTitle = '';
		confirmMessage = '';
		confirmButtonText = '';
		confirmButtonVariant = 'danger';
	}

	async function handleConfirm() {
		if (confirmAction) {
			await confirmAction();
		}
		closeConfirmDialog();
	}

	function deleteUser(userId, userName) {
		showConfirm(
			t('users.deleteUser'),
			t('users.confirmDelete', { name: userName }),
			t('users.deleteUser'),
			async () => {
				try {
					await api.deleteUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || t('users.failedToDelete');
					error = errorMsg;
					errorToast(errorMsg);
				}
			}
		);
	}

	function activateUser(userId, userName) {
		showConfirm(
			t('users.activateUser'),
			t('users.confirmActivate', { name: userName }),
			t('users.activateUser'),
			async () => {
				try {
					await api.activateUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || t('users.failedToActivate');
					error = errorMsg;
					errorToast(errorMsg);
				}
			},
			'primary'
		);
	}

	function deactivateUser(userId, userName) {
		showConfirm(
			t('users.deactivateUser'),
			t('users.confirmDeactivate', { name: userName }),
			t('users.deactivateUser'),
			async () => {
				try {
					await api.deactivateUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || t('users.failedToDeactivate');
					error = errorMsg;
					errorToast(errorMsg);
				}
			},
			'warning'
		);
	}

	function resetUserPassword(userId, userName) {
		resetPasswordUser = { id: userId, name: userName };
		newPassword = '';
		generateRandomPassword = true;
		showPasswordResetModal = true;
	}

	async function performPasswordReset() {
		try {
			const payload = generateRandomPassword 
				? { generate_random: true }
				: { password: newPassword };
			
			const response = await api.resetUserPassword(resetPasswordUser.id, payload);
			
			if (generateRandomPassword) {
				temporaryPassword = response.temporary_password;
			} else {
				temporaryPassword = '';
			}
			
			passwordResetSuccess = true;
			resetPasswordUserName = resetPasswordUser.name;
			closePasswordResetModal();
			showPasswordResultModal = true;
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToResetPassword');
			error = errorMsg;
			errorToast(errorMsg);
		}
	}

	function closePasswordResetModal() {
		showPasswordResetModal = false;
		resetPasswordUser = null;
		newPassword = '';
		generateRandomPassword = true;
	}

	function closePasswordResultModal() {
		showPasswordResultModal = false;
		temporaryPassword = '';
		passwordResetSuccess = false;
		resetPasswordUserName = '';
	}

	function buildUserDropdownItems(user) {
		const isCurrentUser = authStore.currentUser?.id === user.id;

		const items = [
			{
				id: 'edit',
				type: 'regular',
				icon: Edit,
				title: t('common.edit'),
				hoverClass: 'hover-bg',
				onClick: () => editUser(user)
			},
			{
				id: 'reset-password',
				type: 'regular',
				icon: RotateCcw,
				title: t('auth.resetPassword'),
				hoverClass: 'hover-bg',
				onClick: () => resetUserPassword(user.id, user.full_name)
			}
		];

		// Don't show activate/deactivate for current user
		if (!isCurrentUser) {
			// Add activate or deactivate button based on user status
			if (user.is_active) {
				items.push({
					id: 'deactivate',
					type: 'regular',
					icon: UserX,
					title: t('common.disable'),
					color: '#f59e0b',
					hoverClass: 'hover:bg-orange-50',
					onClick: () => deactivateUser(user.id, user.full_name)
				});
			} else {
				items.push({
					id: 'activate',
					type: 'regular',
					icon: UserCheck,
					title: t('common.enable'),
					color: '#10b981',
					hoverClass: 'hover:bg-green-50',
					onClick: () => activateUser(user.id, user.full_name)
				});
			}

			// Add delete as last item (only for non-current users)
			items.push({
				id: 'delete',
				type: 'regular',
				icon: Trash2,
				title: t('common.delete'),
				color: '#dc2626',
				hoverClass: 'hover:bg-red-50',
				onClick: () => deleteUser(user.id, user.full_name)
			});
		}

		return items;
	}

	// Table column definitions
	const userColumns = $derived([
		{
			key: 'name',
			label: t('common.name'),
			slot: 'name'
		},
		{
			key: 'email',
			label: t('common.email')
		},
		{
			key: 'username',
			label: t('common.username'),
			textColor: 'var(--ds-text-subtle)'
		},
		{
			key: 'is_active',
			label: t('common.status'),
			slot: 'status'
		},
		{
			key: 'actions',
			label: t('common.actions')
		}
	]);

	function resetForm() {
		formData = {
			email: '',
			username: '',
			first_name: '',
			last_name: '',
			password: '',
			is_active: true
		};
		editingUser = null;
		showCreateForm = false;
	}

	function editUser(user) {
		formData = {
			email: user.email,
			username: user.username,
			first_name: user.first_name,
			last_name: user.last_name,
			is_active: user.is_active
		};
		editingUser = user;
		showCreateForm = true;
	}

	// Filter users based on search query
	const filteredUsers = $derived(users.filter(user => {
		if (!searchQuery.trim()) return true;

		const query = searchQuery.toLowerCase();
		return (
			user.full_name?.toLowerCase().includes(query) ||
			user.email?.toLowerCase().includes(query) ||
			user.username?.toLowerCase().includes(query)
		);
	}));

	onMount(() => {
		loadUsers();
	});
</script>

<div class="space-y-6">
	<PageHeader
		icon={Users}
		title={t('users.title')}
		subtitle={t('users.subtitle')}
	>
		{#snippet actions()}
			<div class="flex gap-3">
				<SearchInput
					bind:value={searchQuery}
					placeholder={t('users.searchUsers')}
					class="w-64"
				/>
				<Button
					variant="primary"
					icon={Plus}
					onclick={() => {
						resetForm();
						showCreateForm = true;
					}}
					keyboardHint="A"
					hotkeyConfig={{ key: toHotkeyString('users', 'add'), guard: () => !showCreateForm }}
				>
					{t('users.addUser')}
				</Button>
			</div>
		{/snippet}
	</PageHeader>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<Modal isOpen={showCreateForm} onclose={resetForm} maxWidth="max-w-2xl">
		<ModalHeader
			title={editingUser ? t('users.editUser') : t('users.createUser')}
			onClose={resetForm}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			<form onsubmit={(e) => { e.preventDefault(); saveUser(); }} class="space-y-4">
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<Label for="first_name" color="default">{t('users.firstName')}</Label>
						<Input
							id="first_name"
							bind:value={formData.first_name}
							required
						/>
					</div>

					<div>
						<Label for="last_name" color="default">{t('users.lastName')}</Label>
						<Input
							id="last_name"
							bind:value={formData.last_name}
							required
						/>
					</div>
				</div>

				<div>
					<Label for="email" color="default">{t('common.email')}</Label>
					<Input
						id="email"
						type="email"
						bind:value={formData.email}
						required
					/>
				</div>

				<div>
					<Label for="username" color="default">{t('common.username')}</Label>
					<Input
						id="username"
						bind:value={formData.username}
						required
					/>
				</div>

				{#if !editingUser}
					<div>
						<Label for="password" color="default" required>{t('common.password')}</Label>
						<Input
							id="password"
							type="password"
							bind:value={formData.password}
							required
							placeholder={t('placeholders.enterPassword')}
						/>
					</div>
				{/if}
			</form>
		</div>

		<DialogFooter
			confirmLabel={editingUser ? t('common.update') : t('common.create')}
			onCancel={resetForm}
			onConfirm={saveUser}
		/>
	</Modal>

	<Modal isOpen={showPasswordResetModal} onclose={closePasswordResetModal} maxWidth="max-w-md">
		<ModalHeader
			title={t('auth.resetPassword')}
			onClose={closePasswordResetModal}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			<div class="space-y-4">
				<div>
					<label class="flex items-center">
						<input
							type="radio"
							bind:group={generateRandomPassword}
							value={true}
							class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300"
						/>
						<span class="ml-2 text-sm" style="color: var(--ds-text)">{t('auth.resetPassword')}</span>
					</label>
				</div>

				<div>
					<label class="flex items-center">
						<input
							type="radio"
							bind:group={generateRandomPassword}
							value={false}
							class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300"
						/>
						<span class="ml-2 text-sm" style="color: var(--ds-text)">{t('common.custom')}</span>
					</label>
				</div>

				{#if !generateRandomPassword}
					<div class="ml-6">
						<Label for="new-password" color="default" class="mb-1">{t('auth.newPassword')}</Label>
						<input
							id="new-password"
							type="password"
							bind:value={newPassword}
							required={!generateRandomPassword}
							placeholder={t('placeholders.enterNewPassword')}
							class="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						/>
					</div>
				{/if}
			</div>
		</div>

		<DialogFooter
			confirmLabel={t('auth.resetPassword')}
			disabled={!generateRandomPassword && !newPassword.trim()}
			onCancel={closePasswordResetModal}
			onConfirm={performPasswordReset}
		/>
	</Modal>

	<Modal isOpen={showPasswordResultModal} onclose={closePasswordResultModal} maxWidth="max-w-md">
		<ModalHeader
			title={t('toast.success')}
			icon={CheckCircle}
			onClose={closePasswordResultModal}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			{#if temporaryPassword}
				<div class="space-y-3">
					<p class="text-sm" style="color: var(--ds-text-subtle)">
						{t('auth.resetPassword')} - <strong>{resetPasswordUserName}</strong>
					</p>

					<div class="rounded p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border)">
						<div class="flex items-center gap-2 mb-2">
							<Key class="w-4 h-4" style="color: var(--ds-text-subtle)" />
							<span class="text-sm font-medium" style="color: var(--ds-text)">{t('common.password')}</span>
						</div>
						<div class="flex items-center gap-2">
							<code class="flex-1 border rounded px-3 py-2 text-sm font-mono select-all" style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text)">
								{temporaryPassword}
							</code>
							<button
								class="p-2 rounded transition-colors"
								style="color: var(--ds-text-subtle)"
								title={t('common.copy')}
								onclick={() => navigator.clipboard.writeText(temporaryPassword)}
							>
								<Copy class="w-4 h-4" />
							</button>
						</div>
					</div>
				</div>
			{:else}
				<p class="text-sm" style="color: var(--ds-text-subtle)">
					{t('auth.resetPassword')} - <strong>{resetPasswordUserName}</strong>
				</p>
			{/if}
		</div>

		<DialogFooter
			showCancel={false}
			confirmLabel={t('common.done')}
			onConfirm={closePasswordResultModal}
		/>
	</Modal>

	<!-- Confirmation Dialog -->
	<Modal isOpen={showConfirmDialog} onclose={closeConfirmDialog} maxWidth="max-w-md">
		<ModalHeader
			title={confirmTitle}
			icon={AlertTriangle}
			onClose={closeConfirmDialog}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			<p class="text-sm" style="color: var(--ds-text-subtle)">
				{confirmMessage}
			</p>
		</div>

		<DialogFooter
			confirmLabel={confirmButtonText}
			variant={confirmButtonVariant}
			onCancel={closeConfirmDialog}
			onConfirm={handleConfirm}
		/>
	</Modal>

	{#if loading}
		<div class="text-center py-8">
			<div style="color: var(--ds-text-subtle)">{t('common.loading')}</div>
		</div>
	{:else}
		<DataTable
			columns={userColumns}
			data={filteredUsers}
			keyField="id"
			emptyMessage={searchQuery ? t('common.noResults') : t('users.noUsers')}
			emptyIcon={Circle}
			actionItems={buildUserDropdownItems}
		>
			<div slot="name" let:item={user} class="flex items-center">
				{#if user.avatar_url}
					<img class="h-10 w-10 rounded-full" src={user.avatar_url} alt="" />
				{:else}
					<div class="h-10 w-10 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral)">
						<span class="text-sm font-medium" style="color: var(--ds-text)">
							{user.first_name.charAt(0)}{user.last_name.charAt(0)}
						</span>
					</div>
				{/if}
				<div class="ml-4">
					<div class="text-sm font-medium" style="color: var(--ds-text)">
						{user.full_name}
					</div>
					<div class="text-sm" style="color: var(--ds-text-subtle)">
						{t('common.created')} {new Date(user.created_at).toLocaleDateString()}
					</div>
				</div>
			</div>

			<Lozenge slot="status" let:item={user} color={user.is_active ? 'green' : 'red'} text={user.is_active ? t('common.active') : t('common.inactive')} />
		</DataTable>
	{/if}
</div>
