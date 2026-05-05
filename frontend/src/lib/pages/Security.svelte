<script>
	import { onMount } from 'svelte';
	import { currentRoute, navigate } from '../router.js';
	import { authStore, securityStore } from '../stores';
	import { t } from '../stores/i18n.svelte.js';
	import { User, Shield, Key, Smartphone, Plus, Trash2, Code, Copy, Terminal, AlertTriangle, X } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import EmptyState from '../components/EmptyState.svelte';
	import SectionHeader from '../layout/SectionHeader.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import { confirm } from '../composables/useConfirm.js';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import Textarea from '../components/Textarea.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import Label from '../components/Label.svelte';
	import { copyToClipboard } from '../utils/clipboard.js';
	import { formatDate, formatDateShort } from '../utils/dateFormatter.js';
	import { errorToast, successToast } from '../stores/toasts.svelte.js';
	import Checkbox from '../components/Checkbox.svelte';
	import DescriptionText from '../components/DescriptionText.svelte';
	import {
		isWebAuthnSupported,
		prepareCredentialCreationOptions,
		processCredentialCreationResponse
	} from '../utils/webauthn-utils.js';

	// Bind to store values
	let user = $derived(securityStore.user);
	let credentials = $derived(securityStore.credentials);
	let apiTokens = $derived(securityStore.apiTokens);
	let loading = $derived(securityStore.loading);
	let showAddCredential = $derived(securityStore.showAddCredential);
	let credentialType = $derived(securityStore.credentialType);
	let enrollingFIDO = $derived(securityStore.enrollingFIDO);
	let newCredentialName = $derived(securityStore.newCredentialName);
	let newSSHPublicKey = $derived(securityStore.newSSHPublicKey);
	let showAddToken = $derived(securityStore.showAddToken);
	let creatingToken = $derived(securityStore.creatingToken);
	let newTokenName = $derived(securityStore.newTokenName);
	let newTokenExpiry = $derived(securityStore.newTokenExpiry);
	let showNewToken = $derived(securityStore.showNewToken);
	let newTokenValue = $derived(securityStore.newTokenValue);
	let newTokenScopes = $derived(securityStore.newTokenScopes);
	let sshAvailable = $derived(securityStore.sshAvailable);
	let showEnrollmentBanner = $derived(securityStore.showEnrollmentBanner);
	let enrollmentType = $derived(securityStore.enrollmentType);
	let showChangePassword = $derived(securityStore.showChangePassword);
	let changePasswordData = $derived(securityStore.changePasswordData);
	let changePasswordLoading = $derived(securityStore.changePasswordLoading);
	let changePasswordError = $derived(securityStore.changePasswordError);
	let changePasswordSuccess = $derived(securityStore.changePasswordSuccess);

	// Derived from auth store
	let currentUserId = $derived(authStore.currentUser?.id);
	let isSystemAdmin = $derived(authStore.currentUser?.is_system_admin === true);

	// Scope rows for the token permission grid. `resource` drives the
	// `<resource>:<action>` scope string; entries marked read-only have no
	// write/delete scopes on the backend.
	const SCOPE_ROWS = [
		{ resource: 'items', label: 'Items', actions: ['read', 'write', 'delete'] },
		{ resource: 'workspaces', label: 'Workspaces', actions: ['read', 'write', 'delete'] },
		{ resource: 'milestones', label: 'Milestones', actions: ['read', 'write', 'delete'] },
		{ resource: 'iterations', label: 'Iterations', actions: ['read', 'write', 'delete'] },
		{ resource: 'projects', label: 'Projects', actions: ['read', 'write', 'delete'] },
		{ resource: 'users', label: 'Users', actions: ['read'] },
		{ resource: 'statuses', label: 'Statuses', actions: ['read'] },
		{ resource: 'workflows', label: 'Workflows', actions: ['read'] },
		{ resource: 'item-types', label: 'Item types', actions: ['read'] },
		{ resource: 'priorities', label: 'Priorities', actions: ['read'] },
		{ resource: 'custom-fields', label: 'Custom fields', actions: ['read'] },
		{ resource: 'mcp', label: 'MCP server', actions: ['access'] },
	];

	const ADMIN_SCOPE_ROWS = [
		{ resource: 'admin:users', label: 'Users', actions: ['read', 'write'] },
		{ resource: 'admin:groups', label: 'Groups', actions: ['read', 'write'] },
		{ resource: 'admin:audit-logs', label: 'Audit logs', actions: ['read'] },
		{ resource: 'admin:api-tokens', label: 'API tokens', actions: ['read', 'write'] },
	];

	const READ_ONLY_PRESET = [
		'items:read', 'workspaces:read', 'milestones:read', 'iterations:read',
		'projects:read', 'users:read', 'statuses:read', 'workflows:read',
		'item-types:read', 'priorities:read', 'custom-fields:read',
	];

	const READ_WRITE_PRESET = [
		...READ_ONLY_PRESET,
		'items:write', 'workspaces:write', 'milestones:write',
		'iterations:write', 'projects:write',
	];

	function isScopeSelected(scope) {
		return newTokenScopes.includes(scope);
	}

	function toggleScope(scope, checked) {
		const next = new Set(securityStore.newTokenScopes);
		if (checked) next.add(scope); else next.delete(scope);
		securityStore.newTokenScopes = [...next];
	}

	function applyScopePreset(preset) {
		securityStore.newTokenScopes = [...preset];
	}

	function clearScopes() {
		securityStore.newTokenScopes = [];
	}

	// Check for enrollment query parameter
	onMount(() => {
		const unsubscribe = currentRoute.subscribe(route => {
			if (route.query?.enroll === 'passkey') {
				securityStore.checkEnrollmentRequired('passkey');
			}
		});
		return unsubscribe;
	});

	// Initialize when user ID becomes available
	$effect(() => {
		if (currentUserId) {
			securityStore.setCurrentUserId(currentUserId);
		}
	});

	function dismissEnrollmentBanner() {
		securityStore.dismissEnrollmentBanner();
		navigate('/security');
	}

	// Security credential functions
	async function startFIDORegistration() {
		if (!isWebAuthnSupported()) {
			errorToast('WebAuthn is not supported by this browser');
			return;
		}

		try {
			const result = await securityStore.startFIDORegistration(
				prepareCredentialCreationOptions,
				processCredentialCreationResponse
			);

			if (result.wasEnrollmentRequired) {
				successToast('Passkey registered successfully! Your account is now secured.');
			}
		} catch (err) {
			errorToast(err.message || 'Failed to register security key');
		}
	}

	async function createSSHKey() {
		try {
			await securityStore.createSSHKey();
		} catch (err) {
			errorToast(err.message || 'Failed to add SSH key');
		}
	}

	async function confirmRemoveCredential(credentialId, credentialName) {
		const ok = await confirm({
			title: 'Remove Security Credential',
			message: `Are you sure you want to remove the security credential "${credentialName}"? This action cannot be undone.`,
			confirmText: 'Delete',
			variant: 'danger',
		});
		if (!ok) return;
		try {
			await securityStore.removeCredential(credentialId);
		} catch (err) {
			errorToast(err.message || 'Failed to remove credential');
		}
	}

	async function confirmRevokeApiToken(tokenId, tokenName) {
		const ok = await confirm({
			title: 'Revoke API Token',
			message: `Are you sure you want to revoke the token "${tokenName}"? This action cannot be undone and will immediately invalidate the token.`,
			confirmText: 'Delete',
			variant: 'danger',
		});
		if (!ok) return;
		try {
			await securityStore.revokeApiToken(tokenId);
		} catch (err) {
			errorToast(err.message || 'Failed to revoke token');
		}
	}

	async function createApiToken() {
		try {
			await securityStore.createApiToken();
		} catch (err) {
			errorToast(err.message || 'Failed to create token');
		}
	}

	async function handleChangePassword() {
		const result = await securityStore.changePassword();
		if (!result.success && result.error) {
			// Error is already stored in securityStore.changePasswordError
		}
	}

	function cancelChangePassword() {
		securityStore.closeChangePasswordModal();
	}

	function getCredentialIcon(type) {
		switch (type) {
			case 'fido':
				return Key;
			case 'totp':
				return Smartphone;
			case 'ssh':
				return Terminal;
			default:
				return Shield;
		}
	}

	function getCredentialTypeName(type) {
		switch (type) {
			case 'fido':
				return 'Security Key (FIDO2)';
			case 'totp':
				return 'Authenticator App (TOTP)';
			case 'ssh':
				return 'SSH Key';
			default:
				return 'Unknown';
		}
	}

	// Form value setters
	function setCredentialType(value) {
		securityStore.credentialType = value;
	}

	function setNewCredentialName(value) {
		securityStore.newCredentialName = value;
	}

	function setNewSSHPublicKey(value) {
		securityStore.newSSHPublicKey = value;
	}

	function setNewTokenName(value) {
		securityStore.newTokenName = value;
	}

	function setNewTokenExpiry(value) {
		securityStore.newTokenExpiry = value;
	}

	function setChangePasswordData(field, value) {
		securityStore.changePasswordData[field] = value;
	}
</script>

<div class="max-w-4xl mx-auto space-y-6">
	<PageHeader icon={Shield} title={t('security.title')} subtitle={t('security.subtitle')} />

	<!-- Enrollment Banner -->
	{#if showEnrollmentBanner}
		<div class="rounded-lg p-4 border" style="background-color: var(--ds-background-warning-bold); border-color: var(--ds-border-warning);">
			<div class="flex items-start justify-between">
				<div class="flex items-start gap-3">
					<AlertTriangle class="w-6 h-6 flex-shrink-0 mt-0.5" style="color: var(--ds-text-warning-inverse);" />
					<div>
						<h3 class="font-semibold" style="color: var(--ds-text-warning-inverse);">
							Passkey Enrollment Required
						</h3>
						<p class="text-sm mt-1" style="color: var(--ds-text-warning-inverse); opacity: 0.9;">
							{#if enrollmentType === 'passkey'}
								Your organization requires passkey authentication. Please register a security key or use your device's built-in authenticator (like Touch ID or Windows Hello) to continue using this account.
							{:else}
								Please complete your security enrollment to continue.
							{/if}
						</p>
						<div class="mt-3">
							<Button
								variant="default"
								size="small"
								icon={Key}
								onclick={() => { securityStore.credentialType = 'fido'; securityStore.showAddCredential = true; }}
							>
								Register Passkey Now
							</Button>
						</div>
					</div>
				</div>
				<button
					type="button"
					onclick={dismissEnrollmentBanner}
					class="p-1 rounded hover:bg-black/10"
					style="color: var(--ds-text-warning-inverse);"
				>
					<X class="w-5 h-5" />
				</button>
			</div>
		</div>
	{/if}

	<!-- Security Credentials -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<SectionHeader title={t('security.credentials')} subtitle={t('security.credentialsSubtitle')} class="mb-6">
			{#snippet actions()}
				<Button
					variant="primary"
					onclick={() => securityStore.showAddCredential = true}
					icon={Plus}
					size="medium"
					keyboardHint="A"
				>
					{t('common.add')}
				</Button>
			{/snippet}
		</SectionHeader>

		<!-- Credentials List -->
		<div class="space-y-3">
			{#each credentials as credential}
				{@const CredIcon = getCredentialIcon(credential.credential_type)}
				<div class="flex items-center justify-between p-4 border rounded hover-bg" style="border-color: var(--ds-border);">
					<div class="flex items-center space-x-3">
						<CredIcon class="h-6 w-6" style="color: var(--ds-icon-subtle);" />
						<div>
							<div class="font-medium" style="color: var(--ds-text);">{credential.name}</div>
							<div class="text-sm" style="color: var(--ds-text-subtle);">
								{getCredentialTypeName(credential.credential_type)} • Added {formatDateShort(credential.created_at) || '-'}
							</div>
						</div>
					</div>
					<Button
						variant="default"
						size="small"
						icon={Trash2}
						onclick={() => confirmRemoveCredential(credential.id, credential.name)}
					>
						{t('common.remove')}
					</Button>
				</div>
			{:else}
				<EmptyState
					icon={Shield}
					title="No security credentials"
					description="Add a security key or authenticator app to secure your account."
				/>
			{/each}
		</div>
	</div>

	<!-- Account Security -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Account Security</h2>
		<div class="space-y-4">
			<div class="flex items-center justify-between">
				<div>
					<div class="font-medium" style="color: var(--ds-text);">Password</div>
					<div class="text-sm" style="color: var(--ds-text-subtle);">Last updated: Unknown</div>
				</div>
				<Button variant="link" onclick={() => securityStore.showChangePassword = true}>
					Change Password
				</Button>
			</div>
		</div>
	</div>

	<!-- App Tokens -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<SectionHeader title={t('security.apiTokens')} subtitle={t('security.apiTokensSubtitle')} class="mb-6">
			{#snippet actions()}
				<Button
					variant="primary"
					onclick={() => securityStore.showAddToken = true}
					icon={Plus}
					size="medium"
					keyboardHint="T"
				>
					{t('security.createToken')}
				</Button>
			{/snippet}
		</SectionHeader>

		<!-- Show New Token -->
		{#if showNewToken}
			<div class="p-4 rounded mb-6" style="background-color: var(--ds-background-success-subtle); border: 1px solid var(--ds-border-success);">
				<h3 class="text-lg font-medium mb-2" style="color: var(--ds-text-success);">{t('security.tokenCreated')}</h3>
				<p class="text-sm mb-4" style="color: var(--ds-text);">
					{t('security.tokenWarning')}
				</p>
				<div class="flex items-center space-x-2">
					<input
						type="text"
						value={newTokenValue}
						readonly
						class="flex-1 px-3 py-2 rounded font-mono text-sm"
						style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border-success); color: var(--ds-text);"
					/>
					<Button
						variant="default"
						size="small"
						icon={Copy}
						onclick={() => copyToClipboard(newTokenValue)}
					>
						{t('common.copy')}
					</Button>
				</div>
				<div class="mt-3">
					<Button
						variant="default"
						size="small"
						onclick={() => securityStore.closeNewTokenDisplay()}
					>
						{t('common.done')}
					</Button>
				</div>
			</div>
		{/if}

		<!-- Tokens List -->
		<div class="space-y-3">
			{#each apiTokens as token}
				<div class="flex items-center justify-between p-4 border rounded hover-bg" style="border-color: var(--ds-border);">
					<div class="flex items-center space-x-3">
						<Code class="h-6 w-6" style="color: var(--ds-icon-subtle);" />
						<div>
							<div class="font-medium" style="color: var(--ds-text);">{token.name}</div>
							<div class="text-sm" style="color: var(--ds-text-subtle);">
								Created {formatDateShort(token.created_at) || '-'} • Expires {formatDate(token.expires_at) || 'Never expires'}
							</div>
						</div>
					</div>
					<Button
						variant="default"
						size="small"
						icon={Trash2}
						onclick={() => confirmRevokeApiToken(token.id, token.name)}
					>
						{t('security.revokeToken')}
					</Button>
				</div>
			{:else}
				<EmptyState
					icon={Code}
					title="No API tokens"
					description="Generate your first API token to access your account programmatically."
				/>
			{/each}
		</div>
	</div>
</div>

<!-- Change Password Modal -->
<Modal isOpen={showChangePassword} onclose={cancelChangePassword} maxWidth="max-w-md">
	<ModalHeader title={t('auth.changePassword')} onClose={cancelChangePassword} />

	<div class="px-6 py-4">
		{#if changePasswordError}
			<div class="mb-4 p-3 rounded text-sm" style="background-color: var(--ds-background-danger-subtle); border: 1px solid var(--ds-border-danger); color: var(--ds-text-danger);">
				{changePasswordError}
			</div>
		{/if}

		{#if changePasswordSuccess}
			<AlertBox variant="success" message="Password changed successfully!" />
		{:else}
			<div class="space-y-4">
				<div>
					<Label for="current-password" color="default" class="mb-1">{t('auth.currentPassword')}</Label>
					<input
						id="current-password"
						type="password"
						value={changePasswordData.current_password}
						oninput={(e) => setChangePasswordData('current_password', /** @type {HTMLInputElement} */ (e.target).value)}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder={t('placeholders.enterPassword')}
					/>
				</div>

				<div>
					<Label for="new-password" color="default" class="mb-1">{t('auth.newPassword')}</Label>
					<input
						id="new-password"
						type="password"
						value={changePasswordData.new_password}
						oninput={(e) => setChangePasswordData('new_password', /** @type {HTMLInputElement} */ (e.target).value)}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder={t('placeholders.enterNewPassword')}
					/>
				</div>

				<div>
					<Label for="confirm-password" color="default" class="mb-1">{t('auth.confirmPassword')}</Label>
					<input
						id="confirm-password"
						type="password"
						value={changePasswordData.confirm_password}
						oninput={(e) => setChangePasswordData('confirm_password', /** @type {HTMLInputElement} */ (e.target).value)}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder={t('placeholders.confirmNewPassword')}
					/>
				</div>

				<Checkbox
					checked={changePasswordData.logout_all}
					onchange={(checked) => setChangePasswordData('logout_all', checked)}
					label="Log out of all other sessions"
					size="small"
				/>
			</div>
		{/if}
	</div>

	{#if !changePasswordSuccess}
		<DialogFooter
			cancelLabel={t('common.cancel')}
			confirmLabel={t('auth.changePassword')}
			onCancel={cancelChangePassword}
			onConfirm={handleChangePassword}
			disabled={changePasswordLoading || !changePasswordData.current_password || !changePasswordData.new_password || !changePasswordData.confirm_password}
			loading={changePasswordLoading}
		/>
	{/if}
</Modal>

<!-- Add Credential Modal -->
<Modal isOpen={showAddCredential} onclose={() => securityStore.resetCredentialForm()} maxWidth="max-w-lg">
	<div class="p-6">
		<h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
			Add Security Credential
		</h3>

		<!-- Credential Type Selection -->
		<div class="mb-6">
			<fieldset>
				<Label color="default" class="mb-2">Credential Type</Label>
				<div class="flex space-x-4">
					<label class="flex items-center cursor-pointer">
						<input
							type="radio"
							checked={credentialType === 'fido'}
							onchange={() => setCredentialType('fido')}
							class="mr-2"
						/>
						<Key class="h-4 w-4 mr-2" />
						<span style="color: var(--ds-text);">Security Key (FIDO2)</span>
					</label>
					{#if sshAvailable}
					<label class="flex items-center cursor-pointer">
						<input
							type="radio"
							checked={credentialType === 'ssh'}
							onchange={() => setCredentialType('ssh')}
							class="mr-2"
						/>
						<Terminal class="h-4 w-4 mr-2" />
						<span style="color: var(--ds-text);">SSH Key</span>
					</label>
				{/if}
				</div>
			</fieldset>
		</div>

		{#if credentialType === 'fido'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				Security keys provide the strongest protection for your account. Use a hardware key like YubiKey or your device's built-in authenticator.
			</p>
		{:else if credentialType === 'ssh'}
			<p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
				SSH keys allow secure command-line access to the server. Paste your public key (usually from ~/.ssh/id_rsa.pub or ~/.ssh/id_ed25519.pub).
			</p>
		{/if}

		<div class="space-y-4">
			<div>
				<Label for="credential-name" color="default" class="mb-1">{credentialType === 'fido' ? 'Security Key Name' : 'SSH Key Name'}</Label>
				<input
					id="credential-name"
					type="text"
					value={newCredentialName}
					oninput={(e) => setNewCredentialName(/** @type {HTMLInputElement} */ (e.target).value)}
					placeholder={credentialType === 'fido' ? 'e.g., YubiKey, iPhone Touch ID' : 'e.g., MacBook Pro, CI Server'}
					class="w-full px-3 py-2 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
					style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
				/>
			</div>

			{#if credentialType === 'ssh'}
				<div>
					<Label for="ssh-public-key" color="default" class="mb-1">Public Key</Label>
					<Textarea
						id="ssh-public-key"
						value={newSSHPublicKey}
						oninput={(e) => setNewSSHPublicKey(/** @type {HTMLTextAreaElement} */ (e.target).value)}
						placeholder="ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAA... or ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAA..."
						rows={4}
						class="font-mono text-sm"
					/>
					<DescriptionText>Generate with: ssh-keygen -t ed25519 -C "your@email.com"</DescriptionText>
				</div>
			{/if}
		</div>

		<div class="mt-6 flex gap-3">
			<Button
				variant="primary"
				onclick={credentialType === 'fido' ? startFIDORegistration : createSSHKey}
				disabled={!newCredentialName.trim() || (credentialType === 'ssh' && !newSSHPublicKey.trim()) || enrollingFIDO || loading}
				keyboardHint="⏎"
			>
				{#if credentialType === 'fido'}
					{enrollingFIDO ? t('common.processing') : 'Register Security Key'}
				{:else}
					{loading ? t('common.processing') : 'Add SSH Key'}
				{/if}
			</Button>
			<Button
				variant="default"
				onclick={() => securityStore.resetCredentialForm()}
				keyboardHint="Esc"
			>
				{t('common.cancel')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Create Token Modal -->
<Modal isOpen={showAddToken} onclose={() => securityStore.resetTokenForm()} maxWidth="max-w-2xl">
	<div class="p-6">
		<h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
			{t('security.createToken')}
		</h3>

		<div class="space-y-5">
			<div>
				<Label for="token-name" color="default" class="mb-1">{t('security.tokenName')}</Label>
				<input
					id="token-name"
					type="text"
					value={newTokenName}
					oninput={(e) => setNewTokenName(/** @type {HTMLInputElement} */ (e.target).value)}
					placeholder="e.g., Mobile App, CI/CD Pipeline"
					class="w-full px-3 py-2 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
					style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
				/>
			</div>

			<div>
				<div class="flex items-center justify-between mb-2">
					<Label color="default" class="mb-0">Permissions</Label>
					<div class="flex gap-2">
						<Button variant="link" size="small" onclick={() => applyScopePreset(READ_ONLY_PRESET)}>Read-only</Button>
						<Button variant="link" size="small" onclick={() => applyScopePreset(READ_WRITE_PRESET)}>Read + Write</Button>
						<Button variant="link" size="small" onclick={clearScopes}>Clear</Button>
					</div>
				</div>
				<DescriptionText>
					Scopes limit what this token can do. The token also inherits your account's workspace permissions.
				</DescriptionText>

				<div class="mt-3 rounded border overflow-hidden" style="border-color: var(--ds-border);">
					<table class="w-full text-sm">
						<thead>
							<tr style="background-color: var(--ds-background-neutral);">
								<th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text-subtle);">Resource</th>
								<th class="px-3 py-2 font-medium w-20" style="color: var(--ds-text-subtle);">Read</th>
								<th class="px-3 py-2 font-medium w-20" style="color: var(--ds-text-subtle);">Write</th>
								<th class="px-3 py-2 font-medium w-20" style="color: var(--ds-text-subtle);">Delete</th>
							</tr>
						</thead>
						<tbody>
							{#each SCOPE_ROWS as row}
								<tr class="border-t" style="border-color: var(--ds-border);">
									<td class="px-3 py-2" style="color: var(--ds-text);">{row.label}</td>
									{#each ['read', 'write', 'delete'] as action}
										<td class="px-3 py-2 text-center">
											{#if row.actions.includes(action)}
												{@const scope = `${row.resource}:${action}`}
												<Checkbox
													checked={isScopeSelected(scope)}
													onchange={(checked) => toggleScope(scope, checked)}
													size="small"
												/>
											{:else}
												<span style="color: var(--ds-text-subtlest);">—</span>
											{/if}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				{#if isSystemAdmin}
					<div class="mt-4">
						<Label color="default" class="mb-2">Admin permissions</Label>
						<DescriptionText>Require the system admin role. Use with care.</DescriptionText>
						<div class="mt-2 rounded border overflow-hidden" style="border-color: var(--ds-border);">
							<table class="w-full text-sm">
								<thead>
									<tr style="background-color: var(--ds-background-neutral);">
										<th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text-subtle);">Resource</th>
										<th class="px-3 py-2 font-medium w-20" style="color: var(--ds-text-subtle);">Read</th>
										<th class="px-3 py-2 font-medium w-20" style="color: var(--ds-text-subtle);">Write</th>
									</tr>
								</thead>
								<tbody>
									{#each ADMIN_SCOPE_ROWS as row}
										<tr class="border-t" style="border-color: var(--ds-border);">
											<td class="px-3 py-2" style="color: var(--ds-text);">{row.label}</td>
											{#each ['read', 'write'] as action}
												<td class="px-3 py-2 text-center">
													{#if row.actions.includes(action)}
														{@const scope = `${row.resource}:${action}`}
														<Checkbox
															checked={isScopeSelected(scope)}
															onchange={(checked) => toggleScope(scope, checked)}
															size="small"
														/>
													{:else}
														<span style="color: var(--ds-text-subtlest);">—</span>
													{/if}
												</td>
											{/each}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				{/if}
			</div>

			<div>
				<Label for="token-expiry" color="default" class="mb-1">Expiration (Optional)</Label>
				<input
					id="token-expiry"
					type="date"
					value={newTokenExpiry}
					oninput={(e) => setNewTokenExpiry(/** @type {HTMLInputElement} */ (e.target).value)}
					min={formatDate(new Date())}
					class="w-full px-3 py-2 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
					style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
				/>
				<DescriptionText>Leave empty for tokens that never expire</DescriptionText>
			</div>
		</div>

		<div class="mt-6 flex gap-3">
			<Button
				variant="primary"
				onclick={createApiToken}
				disabled={!newTokenName.trim() || newTokenScopes.length === 0 || creatingToken}
				keyboardHint="⏎"
			>
				{creatingToken ? t('common.processing') : t('security.createToken')}
			</Button>
			<Button
				variant="default"
				onclick={() => securityStore.resetTokenForm()}
				keyboardHint="Esc"
			>
				{t('common.cancel')}
			</Button>
		</div>
	</div>
</Modal>
