<script>
	import { onMount } from 'svelte';
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { Plus, Edit, Trash2, RotateCcw, Circle, CheckCircle, Key, Users, UserCheck, UserX, AlertTriangle, Send, Link } from '@lucide/svelte';
	import CopyButton from '../components/CopyButton.svelte';
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
	import Checkbox from '../components/Checkbox.svelte';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';
	import { t } from '../stores/i18n.svelte.js';
	import { formatDateSimple } from '../utils/dateFormatter.js';
	import { confirm } from '../composables/useConfirm.js';

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
	let isInviteMode = $state(false);
	let showInviteResultModal = $state(false);
	let invitationLink = $state('');
	let emailSent = $state(false);


	// Form data
	let formData = $state({
		email: '',
		username: '',
		first_name: '',
		last_name: '',
		password: '',
		is_active: true,
		is_agent: false
	});

	// Token minting state for service users / agents
	let showTokenModal = $state(false);
	let tokenTargetUser = $state(null);
	let newTokenName = $state('');
	let newTokenExpiresDays = $state(90);
	let creatingToken = $state(false);
	let mintedToken = $state('');
	let mintedTokenError = $state('');

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
		if (!editingUser && !isInviteMode && !formData.password.trim()) {
			errorToast(t('auth.passwordRequired'));
			return;
		}
		try {
			if (editingUser) {
				await api.updateUser(editingUser.id, formData);
			} else if (isInviteMode) {
				const result = await api.inviteUser(formData);
				invitationLink = `${window.location.origin}/set-password/${result.token}`;
				emailSent = result.email_sent;
				showInviteResultModal = true;
			} else {
				await api.createUser(formData);
			}

			if (!isInviteMode || editingUser) {
				resetForm();
			}
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToSave');
			error = errorMsg;
			errorToast(errorMsg);
		}
	}

	async function deleteUser(userId, userName) {
		const ok = await confirm({
			title: t('users.deleteUser'),
			message: t('users.confirmDelete', { name: userName }),
			confirmText: t('users.deleteUser'),
			variant: 'danger',
		});
		if (!ok) return;
		try {
			await api.deleteUser(userId);
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToDelete');
			error = errorMsg;
			errorToast(errorMsg);
		}
	}

	async function activateUser(userId, userName) {
		const ok = await confirm({
			title: t('users.activateUser'),
			message: t('users.confirmActivate', { name: userName }),
			confirmText: t('users.activateUser'),
			variant: 'primary',
		});
		if (!ok) return;
		try {
			await api.activateUser(userId);
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToActivate');
			error = errorMsg;
			errorToast(errorMsg);
		}
	}

	async function deactivateUser(userId, userName) {
		const ok = await confirm({
			title: t('users.deactivateUser'),
			message: t('users.confirmDeactivate', { name: userName }),
			confirmText: t('users.deactivateUser'),
			variant: 'warning',
		});
		if (!ok) return;
		try {
			await api.deactivateUser(userId);
			await loadUsers();
		} catch (err) {
			const errorMsg = err.message || t('users.failedToDeactivate');
			error = errorMsg;
			errorToast(errorMsg);
		}
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

		// Admins can mint API tokens for agents (service users or user-owned).
		// The backend enforces: admin + target-is-agent OR caller-is-owner.
		if (user.is_agent) {
			items.push({
				id: 'mint-token',
				type: 'regular',
				icon: Key,
				title: 'Mint API token',
				hoverClass: 'hover-bg',
				onClick: () => openTokenModal(user)
			});
		}

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
				color: 'var(--ds-text-danger)',
				hoverClass: 'hover-danger',
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
			is_active: true,
			is_agent: false
		};
		editingUser = null;
		showCreateForm = false;
		isInviteMode = false;
	}

	function startInvite() {
		resetForm();
		isInviteMode = true;
		showCreateForm = true;
	}

	function closeInviteResultModal() {
		showInviteResultModal = false;
		invitationLink = '';
		emailSent = false;
		resetForm();
	}

	function editUser(user) {
		formData = {
			email: user.email,
			username: user.username,
			first_name: user.first_name,
			last_name: user.last_name,
			password: '',
			is_active: user.is_active,
			// is_agent is immutable at the DB level; shown read-only in the edit form.
			is_agent: !!user.is_agent
		};
		editingUser = user;
		showCreateForm = true;
	}

	function openTokenModal(user) {
		tokenTargetUser = user;
		newTokenName = `${user.username}-token`;
		newTokenExpiresDays = 90;
		mintedToken = '';
		mintedTokenError = '';
		showTokenModal = true;
	}

	function closeTokenModal() {
		showTokenModal = false;
		tokenTargetUser = null;
		newTokenName = '';
		mintedToken = '';
		mintedTokenError = '';
	}

	async function createTokenForUser() {
		if (!tokenTargetUser) return;
		mintedTokenError = '';
		creatingToken = true;
		try {
			const payload = {
				name: newTokenName,
				user_id: tokenTargetUser.id,
				// Default scopes for a service-user / agent token. Admins can
				// revoke and mint a narrower one if needed.
				permissions: ['items:read', 'items:write', 'workspaces:read', 'users:read']
			};
			if (newTokenExpiresDays && Number(newTokenExpiresDays) > 0) {
				const expires = new Date();
				expires.setDate(expires.getDate() + Number(newTokenExpiresDays));
				payload.expires_at = expires.toISOString();
			}
			const result = await api.createApiToken(payload);
			mintedToken = result?.token || result?.api_token?.token || '';
		} catch (err) {
			mintedTokenError = err?.message || 'Failed to create token';
		} finally {
			creatingToken = false;
		}
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
					variant="default"
					icon={Send}
					onclick={startInvite}
				>
					{t('auth.inviteUser')}
				</Button>
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
			title={editingUser ? t('users.editUser') : (isInviteMode ? t('auth.inviteUser') : t('users.createUser'))}
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

				{#if !editingUser && !isInviteMode}
					<div>
						<Label for="password" color="default" {...formData.is_agent ? {} : { required: true }}>
							{formData.is_agent ? `${t('common.password')} (not used for agents)` : t('common.password')}
						</Label>
						<Input
							id="password"
							type="password"
							bind:value={formData.password}
							required={!formData.is_agent}
							disabled={formData.is_agent}
							placeholder={formData.is_agent ? 'Agents authenticate via API token only' : t('placeholders.enterPassword')}
						/>
					</div>

					<div class="flex items-start gap-3 p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);">
						<Checkbox
							bind:checked={formData.is_agent}
							label="Service user (agent)"
							hint="Agents are non-human identities that authenticate via API tokens only — they cannot log in interactively. This flag is permanent and cannot be changed later."
						/>
					</div>
				{/if}

				{#if editingUser && editingUser.is_agent}
					<div class="flex items-center gap-2 p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);">
						<Lozenge appearance="new">Service user (agent)</Lozenge>
						<span class="text-xs" style="color: var(--ds-text-subtle);">
							The agent flag is immutable once set at creation.
						</span>
					</div>
				{/if}
			</form>
		</div>

		<DialogFooter
			confirmLabel={editingUser ? t('common.update') : (isInviteMode ? t('auth.inviteUser') : t('common.create'))}
			onCancel={resetForm}
			onConfirm={saveUser}
		/>
	</Modal>

	<!-- Invite Result Modal -->
	<Modal isOpen={showInviteResultModal} onclose={closeInviteResultModal} maxWidth="max-w-md">
		<ModalHeader
			title={t('auth.inviteSent')}
			icon={CheckCircle}
			onClose={closeInviteResultModal}
		/>

		<div class="px-6 py-4">
			<div class="space-y-4">
				{#if emailSent}
					<div class="flex items-start gap-3 p-3 bg-green-50 rounded-lg text-green-800 border border-green-100">
						<CheckCircle class="w-5 h-5 flex-shrink-0 mt-0.5" />
						<p class="text-sm">An invitation email has been sent to <strong>{formData.email}</strong>.</p>
					</div>
				{:else}
					<div class="flex items-start gap-3 p-3 bg-amber-50 rounded-lg text-amber-800 border border-amber-100">
						<AlertTriangle class="w-5 h-5 flex-shrink-0 mt-0.5" />
						<div>
							<p class="text-sm font-medium">Email could not be sent.</p>
							<p class="text-xs mt-1">SMTP might not be configured. You can manually share the link below with the user.</p>
						</div>
					</div>
				{/if}

				<div class="space-y-2">
					<Label color="default">{t('auth.invitationLink') || 'Invitation Link'}</Label>
					<div class="flex items-center gap-2">
						<code class="flex-1 border rounded px-3 py-2 text-sm font-mono select-all truncate" style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border); color: var(--ds-text)">
							{invitationLink}
						</code>
						<CopyButton text={invitationLink} title={t('common.copy')} />
					</div>
				</div>
			</div>
		</div>

		<DialogFooter
			showCancel={false}
			confirmLabel={t('common.done')}
			onConfirm={closeInviteResultModal}
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
							<CopyButton text={temporaryPassword} title={t('common.copy')} />
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
			{#snippet name(user)}
				<div class="flex items-center">
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
						<div class="text-sm font-medium flex items-center gap-2" style="color: var(--ds-text)">
							{user.full_name}
							{#if user.is_agent}
								<Lozenge
									color="purple"
									text={user.agent_owner_user_id
										? `agent of ${user.agent_owner_name || `#${user.agent_owner_user_id}`}`
										: 'service user'}
								/>
							{/if}
						</div>
						<div class="text-sm" style="color: var(--ds-text-subtle)">
							{t('common.created')} {formatDateSimple(user.created_at)}
						</div>
					</div>
				</div>
			{/snippet}

			{#snippet status(user)}
				<Lozenge color={user.is_active ? 'green' : 'red'} text={user.is_active ? t('common.active') : t('common.inactive')} />
			{/snippet}
		</DataTable>
	{/if}

	<!-- Mint API token for agent -->
	<Modal isOpen={showTokenModal} onclose={closeTokenModal} maxWidth="max-w-md">
		<ModalHeader
			title={tokenTargetUser ? `Mint API token for ${tokenTargetUser.full_name || tokenTargetUser.username}` : 'Mint API token'}
			icon={Key}
			onClose={closeTokenModal}
		/>

		<div class="px-6 py-4 space-y-4">
			{#if mintedToken}
				<div class="space-y-2">
					<p class="text-sm" style="color: var(--ds-text);">
						Copy this token now — you won't be able to see it again.
					</p>
					<div class="flex items-center gap-2">
						<code class="flex-1 border rounded px-3 py-2 text-sm font-mono select-all break-all" style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border); color: var(--ds-text);">
							{mintedToken}
						</code>
						<CopyButton text={mintedToken} title={t('common.copy')} />
					</div>
				</div>
			{:else}
				<div>
					<Label for="token-name" color="default" required>Token name</Label>
					<Input id="token-name" bind:value={newTokenName} required />
				</div>
				<div>
					<Label for="token-expiry" color="default">Expires in (days, 0 = never)</Label>
					<Input id="token-expiry" type="number" min="0" max="3650" bind:value={newTokenExpiresDays} />
				</div>
				{#if mintedTokenError}
					<AlertBox message={mintedTokenError} />
				{/if}
			{/if}
		</div>

		<DialogFooter
			confirmLabel={mintedToken ? t('common.done') : 'Mint token'}
			onCancel={closeTokenModal}
			onConfirm={mintedToken ? closeTokenModal : createTokenForUser}
			confirmDisabled={creatingToken}
		/>
	</Modal>
</div>
