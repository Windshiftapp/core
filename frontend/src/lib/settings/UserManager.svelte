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
	import { matchesShortcut } from '../utils/keyboardShortcuts.js';

	let users = [];
	let loading = false;
	let error = '';
	let searchQuery = '';
	let showCreateForm = false;
	let editingUser = null;
	let showPasswordResetModal = false;
	let resetPasswordUser = null;
	let newPassword = '';
	let generateRandomPassword = true;
	let showPasswordResultModal = false;
	let temporaryPassword = '';
	let passwordResetSuccess = false;
	let resetPasswordUserName = '';


	// Confirmation dialog state
	let showConfirmDialog = false;
	let confirmAction = null;
	let confirmTitle = '';
	let confirmMessage = '';
	let confirmButtonText = '';
	let confirmButtonVariant = 'danger';

	// Form data
	let formData = {
		email: '',
		username: '',
		first_name: '',
		last_name: '',
		password: '',
		is_active: true
	};

	async function loadUsers() {
		loading = true;
		try {
			users = await api.getUsers();
			error = '';
		} catch (err) {
			error = err.message || 'Failed to load users';
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
			const errorMsg = err.message || 'Failed to save user';
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
			'Delete User',
			`Are you sure you want to delete ${userName}? This action cannot be undone.`,
			'Delete User',
			async () => {
				try {
					await api.deleteUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || 'Failed to delete user';
					error = errorMsg;
					errorToast(errorMsg);
				}
			}
		);
	}

	function activateUser(userId, userName) {
		showConfirm(
			'Activate User',
			`Are you sure you want to activate ${userName}? They will be able to access the system.`,
			'Activate User',
			async () => {
				try {
					await api.activateUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || 'Failed to activate user';
					error = errorMsg;
					errorToast(errorMsg);
				}
			},
			'primary'
		);
	}

	function deactivateUser(userId, userName) {
		showConfirm(
			'Deactivate User',
			`Are you sure you want to deactivate ${userName}? They will no longer be able to access the system.`,
			'Deactivate User',
			async () => {
				try {
					await api.deactivateUser(userId);
					await loadUsers();
				} catch (err) {
					const errorMsg = err.message || 'Failed to deactivate user';
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
			const errorMsg = err.message || 'Failed to reset password';
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
				title: 'Edit',
				hoverClass: 'hover:bg-gray-100',
				onClick: () => editUser(user)
			},
			{
				id: 'reset-password',
				type: 'regular',
				icon: RotateCcw,
				title: 'Reset Password',
				hoverClass: 'hover:bg-gray-100',
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
					title: 'Deactivate',
					color: '#f59e0b',
					hoverClass: 'hover:bg-orange-50',
					onClick: () => deactivateUser(user.id, user.full_name)
				});
			} else {
				items.push({
					id: 'activate',
					type: 'regular',
					icon: UserCheck,
					title: 'Activate',
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
				title: 'Delete',
				color: '#dc2626',
				hoverClass: 'hover:bg-red-50',
				onClick: () => deleteUser(user.id, user.full_name)
			});
		}

		return items;
	}

	// Table column definitions
	const userColumns = [
		{
			key: 'name',
			label: 'Name',
			slot: 'name'
		},
		{
			key: 'email',
			label: 'Email'
		},
		{
			key: 'username',
			label: 'Username',
			textColor: 'var(--ds-text-subtle)'
		},
		{
			key: 'is_active',
			label: 'Status',
			slot: 'status'
		},
		{
			key: 'actions',
			label: 'Actions'
		}
	];

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
	$: filteredUsers = users.filter(user => {
		if (!searchQuery.trim()) return true;

		const query = searchQuery.toLowerCase();
		return (
			user.full_name?.toLowerCase().includes(query) ||
			user.email?.toLowerCase().includes(query) ||
			user.username?.toLowerCase().includes(query)
		);
	});

	onMount(() => {
		loadUsers();

		// Add keyboard shortcuts
		function handleKeydown(event) {
			// 'a' key to open create user dialog
			if (matchesShortcut(event, { key: 'a' })) {
				// Don't trigger if we're in an input/textarea
				if (event.target.tagName !== 'INPUT' && event.target.tagName !== 'TEXTAREA') {
					event.preventDefault();
					resetForm();
					showCreateForm = true;
				}
			}

			// ESC key to close create user dialog
			if (event.key === 'Escape' && showCreateForm) {
				event.preventDefault();
				resetForm();
				showCreateForm = false;
			}

			// Enter key to save user when in create dialog
			if (event.key === 'Enter' && showCreateForm) {
				// Don't trigger if we're in a textarea (allow multi-line input)
				if (event.target.tagName !== 'TEXTAREA') {
					event.preventDefault();
					saveUser();
				}
			}
		}

		document.addEventListener('keydown', handleKeydown);
		return () => document.removeEventListener('keydown', handleKeydown);
	});
</script>

<div class="space-y-6">
	<PageHeader
		icon={Users}
		title="User Management"
		subtitle="Manage user accounts, roles, and access permissions"
	>
		{#snippet actions()}
			<div class="flex gap-3">
				<SearchInput
					bind:value={searchQuery}
					placeholder="Search users..."
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
				>
					Add User
				</Button>
			</div>
		{/snippet}
	</PageHeader>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<Modal isOpen={showCreateForm} onclose={resetForm} maxWidth="max-w-2xl">
		<ModalHeader
			title={editingUser ? 'Edit User' : 'Create New User'}
			onClose={resetForm}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			<form onsubmit={(e) => { e.preventDefault(); saveUser(); }} class="space-y-4">
				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<Label for="first_name" color="default">First Name</Label>
						<Input
							id="first_name"
							bind:value={formData.first_name}
							required
						/>
					</div>

					<div>
						<Label for="last_name" color="default">Last Name</Label>
						<Input
							id="last_name"
							bind:value={formData.last_name}
							required
						/>
					</div>
				</div>

				<div>
					<Label for="email" color="default">Email</Label>
					<Input
						id="email"
						type="email"
						bind:value={formData.email}
						required
					/>
				</div>

				<div>
					<Label for="username" color="default">Username</Label>
					<Input
						id="username"
						bind:value={formData.username}
						required
					/>
				</div>

				{#if !editingUser}
					<div>
						<Label for="password" color="default" required>Password</Label>
						<Input
							id="password"
							type="password"
							bind:value={formData.password}
							required
							placeholder="Enter password"
						/>
						<p class="text-xs mt-1" style="color: var(--ds-text-subtle)">
							Enter a password for the user
						</p>
					</div>
				{/if}
			</form>
		</div>

		<DialogFooter
			confirmLabel={editingUser ? 'Update User' : 'Create User'}
			onCancel={resetForm}
			onConfirm={saveUser}
		/>
	</Modal>

	<Modal isOpen={showPasswordResetModal} onclose={closePasswordResetModal} maxWidth="max-w-md">
		<ModalHeader
			title="Reset Password for {resetPasswordUser?.name}"
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
						<span class="ml-2 text-sm" style="color: var(--ds-text)">Generate random temporary password</span>
					</label>
					<p class="ml-6 text-xs mt-1" style="color: var(--ds-text-subtle)">
						A secure random password will be generated and displayed to copy
					</p>
				</div>

				<div>
					<label class="flex items-center">
						<input
							type="radio"
							bind:group={generateRandomPassword}
							value={false}
							class="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300"
						/>
						<span class="ml-2 text-sm" style="color: var(--ds-text)">Set custom password</span>
					</label>
					<p class="ml-6 text-xs mt-1" style="color: var(--ds-text-subtle)">
						Specify a password for the user
					</p>
				</div>

				{#if !generateRandomPassword}
					<div class="ml-6">
						<Label for="new-password" color="default" class="mb-1">New Password</Label>
						<input
							id="new-password"
							type="password"
							bind:value={newPassword}
							required={!generateRandomPassword}
							placeholder="Enter new password"
							class="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						/>
					</div>
				{/if}
			</div>
		</div>

		<DialogFooter
			confirmLabel="Reset Password"
			disabled={!generateRandomPassword && !newPassword.trim()}
			onCancel={closePasswordResetModal}
			onConfirm={performPasswordReset}
		/>
	</Modal>

	<Modal isOpen={showPasswordResultModal} onclose={closePasswordResultModal} maxWidth="max-w-md">
		<ModalHeader
			title="Password Reset Successful"
			icon={CheckCircle}
			onClose={closePasswordResultModal}
		/>

		<!-- Modal content -->
		<div class="px-6 py-4">
			{#if temporaryPassword}
				<div class="space-y-3">
					<p class="text-sm" style="color: var(--ds-text-subtle)">
						A temporary password has been generated for <strong>{resetPasswordUserName}</strong>.
					</p>

					<div class="rounded p-4 border" style="background-color: var(--ds-surface); border-color: var(--ds-border)">
						<div class="flex items-center gap-2 mb-2">
							<Key class="w-4 h-4" style="color: var(--ds-text-subtle)" />
							<span class="text-sm font-medium" style="color: var(--ds-text)">Temporary Password</span>
						</div>
						<div class="flex items-center gap-2">
							<code class="flex-1 border rounded px-3 py-2 text-sm font-mono select-all" style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text)">
								{temporaryPassword}
							</code>
							<button
								class="p-2 rounded transition-colors"
								style="color: var(--ds-text-subtle)"
								title="Copy to clipboard"
								onclick={() => navigator.clipboard.writeText(temporaryPassword)}
							>
								<Copy class="w-4 h-4" />
							</button>
						</div>
					</div>

					<div class="bg-blue-50 rounded p-3">
						<p class="text-xs text-blue-800">
							<strong>Important:</strong> Please provide this temporary password to the user.
							They will be required to change it on their next login.
						</p>
					</div>
				</div>
			{:else}
				<p class="text-sm" style="color: var(--ds-text-subtle)">
					The new password has been successfully set for <strong>{resetPasswordUserName}</strong>.
				</p>
			{/if}
		</div>

		<DialogFooter
			showCancel={false}
			confirmLabel="Done"
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
			<div style="color: var(--ds-text-subtle)">Loading users...</div>
		</div>
	{:else}
		<DataTable
			columns={userColumns}
			data={filteredUsers}
			keyField="id"
			emptyMessage={searchQuery ? 'No users match your search' : 'No users found'}
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
						Created {new Date(user.created_at).toLocaleDateString()}
					</div>
				</div>
			</div>

			<Lozenge slot="status" let:item={user} color={user.is_active ? 'green' : 'red'} text={user.is_active ? 'Active' : 'Inactive'} />
		</DataTable>
	{/if}
</div>

