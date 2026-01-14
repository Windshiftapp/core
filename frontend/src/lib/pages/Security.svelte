<script>
	import { api } from '../api.js';
	import { authStore } from '../stores';
	import { User, Shield, Key, Smartphone, Plus, Trash2, Calendar, CheckCircle, PlayCircle, Code, Copy, Eye, EyeOff, Terminal } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import SectionHeader from '../layout/SectionHeader.svelte';
	import ConfirmDialog from '../dialogs/ConfirmDialog.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import Textarea from '../components/Textarea.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import Label from '../components/Label.svelte';
	import { formatDate, formatDateShort } from '../utils/dateFormatter.js';
	import { errorToast } from '../stores/toasts.svelte.js';
	import {
		isWebAuthnSupported,
		prepareCredentialCreationOptions,
		processCredentialCreationResponse
	} from '../utils/webauthn-utils.js';

	// State variables
	let user = $state(null);
	let credentials = $state([]);
	let apiTokens = $state([]);
	let loading = $state(false);
	let showAddCredential = $state(false);
	let credentialType = $state('fido'); // 'fido' or 'ssh'
	let enrollingFIDO = $state(false);
	let newCredentialName = $state('');
	let newSSHPublicKey = $state('');
	let testingLogin = $state(false);
	let loginTestResult = $state('');
	let showAddToken = $state(false);
	let creatingToken = $state(false);
	let newTokenName = $state('');
	let newTokenScopes = $state([]);
	let newTokenExpiry = $state('');
	let showNewToken = $state(false);
	let newTokenValue = $state('');
	let showConfirmDialog = $state(false);
	let confirmDialogConfig = $state({
		title: '',
		message: '',
		action: null
	});

	// Change password state
	let showChangePassword = $state(false);
	let changePasswordData = $state({
		current_password: '',
		new_password: '',
		confirm_password: '',
		logout_all: false
	});
	let changePasswordLoading = $state(false);
	let changePasswordError = $state('');
	let changePasswordSuccess = $state(false);

	// Use current user ID from auth store
	let currentUserId = $derived(authStore.currentUser?.id);

	// Initialize data when currentUserId becomes available
	let initialized = $state(false);

	$effect(() => {
		if (currentUserId && !initialized) {
			initialized = true;
			loadUserProfile();
			loadCredentials();
			loadApiTokens();
		}
	});

	async function loadUserProfile() {
		if (!currentUserId) return;
		try {
			loading = true;
			user = await api.getUser(currentUserId);
		} catch (err) {
			errorToast(err.message || 'Failed to load user profile');
		} finally {
			loading = false;
		}
	}

	async function loadCredentials() {
		if (!currentUserId) return;
		try {
			credentials = await api.getUserCredentials(currentUserId);
		} catch (err) {
			console.warn('Failed to load credentials:', err);
			credentials = [];
		}
	}

	async function loadApiTokens() {
		try {
			apiTokens = await api.getApiTokens();
		} catch (err) {
			console.warn('Failed to load API tokens:', err);
			apiTokens = [];
		}
	}

	// Security credential functions
	async function startFIDORegistration() {
		if (!currentUserId || !newCredentialName.trim()) return;

		// Check WebAuthn support
		if (!isWebAuthnSupported()) {
			errorToast('WebAuthn is not supported by this browser');
			return;
		}

		try {
			enrollingFIDO = true;

			// Start registration with server
			const registrationData = await api.startFIDORegistration(currentUserId, newCredentialName.trim());

			// Extract session ID and options
			const sessionId = registrationData.sessionId;
			const publicKeyOptions = registrationData.publicKey || registrationData.options || registrationData;

			if (!publicKeyOptions || !publicKeyOptions.challenge) {
				throw new Error('Invalid registration response from server');
			}

			// Prepare options for browser API
			const credentialCreationOptions = prepareCredentialCreationOptions(publicKeyOptions);

			// Create credential using browser API
			const credential = await navigator.credentials.create(credentialCreationOptions);

			// Process credential for server
			const credentialResponse = processCredentialCreationResponse(credential);

			// Complete registration with server
			const completionData = {
				sessionId: sessionId,
				credentialName: newCredentialName.trim(),
				response: credentialResponse
			};

			await api.completeFIDORegistration(currentUserId, completionData);
			await loadCredentials();

			newCredentialName = '';
			showAddCredential = false;
		} catch (err) {
			console.error('FIDO registration error:', err);
			errorToast(err.message || 'Failed to register security key');
		} finally {
			enrollingFIDO = false;
		}
	}

	async function createSSHKey() {
		if (!currentUserId || !newCredentialName.trim() || !newSSHPublicKey.trim()) return;
		
		try {
			loading = true;
			await api.createSSHKey(currentUserId, newCredentialName.trim(), newSSHPublicKey.trim());
			await loadCredentials();
			
			newCredentialName = '';
			newSSHPublicKey = '';
			showAddCredential = false;
		} catch (err) {
			errorToast(err.message || 'Failed to add SSH key');
		} finally {
			loading = false;
		}
	}

	function confirmRemoveCredential(credentialId, credentialName) {
		confirmDialogConfig = {
			title: 'Remove Security Credential',
			message: `Are you sure you want to remove the security credential "${credentialName}"? This action cannot be undone.`,
			action: () => removeCredential(credentialId)
		};
		showConfirmDialog = true;
	}

	async function removeCredential(credentialId) {
		if (!currentUserId) return;
		
		try {
			await api.removeUserCredential(currentUserId, credentialId);
			await loadCredentials();
		} catch (err) {
			errorToast(err.message || 'Failed to remove credential');
		}
	}

	// API token functions
	async function createApiToken() {
		if (!newTokenName.trim()) return;
		
		try {
			creatingToken = true;
			const tokenData = {
				name: newTokenName.trim(),
				permissions: newTokenScopes.length > 0 ? newTokenScopes : ['read'],
				expires_at: newTokenExpiry || null
			};
			
			const result = await api.createApiToken(tokenData);
			newTokenValue = result.token;
			showNewToken = true;
			
			await loadApiTokens();
			resetTokenForm();
		} catch (err) {
			errorToast(err.message || 'Failed to create token');
		} finally {
			creatingToken = false;
		}
	}

	function confirmRevokeApiToken(tokenId, tokenName) {
		confirmDialogConfig = {
			title: 'Revoke API Token',
			message: `Are you sure you want to revoke the token "${tokenName}"? This action cannot be undone and will immediately invalidate the token.`,
			action: () => revokeApiToken(tokenId)
		};
		showConfirmDialog = true;
	}

	async function revokeApiToken(tokenId) {
		try {
			await api.revokeApiToken(tokenId);
			await loadApiTokens();
		} catch (err) {
			errorToast(err.message || 'Failed to revoke token');
		}
	}

	function resetTokenForm() {
		newTokenName = '';
		newTokenScopes = [];
		newTokenExpiry = '';
		showAddToken = false;
	}

	function resetCredentialForm() {
		newCredentialName = '';
		newSSHPublicKey = '';
		credentialType = 'fido';
		showAddCredential = false;
	}

	// Confirm dialog handlers
	function handleConfirm() {
		if (confirmDialogConfig.action) {
			confirmDialogConfig.action();
		}
		showConfirmDialog = false;
	}

	function handleCancel() {
		showConfirmDialog = false;
	}

	async function handleChangePassword() {
		changePasswordError = '';

		// Validate passwords match
		if (changePasswordData.new_password !== changePasswordData.confirm_password) {
			changePasswordError = 'New passwords do not match';
			return;
		}

		// Validate minimum length
		if (changePasswordData.new_password.length < 8) {
			changePasswordError = 'Password must be at least 8 characters';
			return;
		}

		changePasswordLoading = true;
		try {
			await api.auth.changePassword({
				current_password: changePasswordData.current_password,
				new_password: changePasswordData.new_password,
				logout_all: changePasswordData.logout_all
			});
			changePasswordSuccess = true;
			// Reset form after brief delay
			setTimeout(() => {
				showChangePassword = false;
				changePasswordSuccess = false;
				changePasswordData = { current_password: '', new_password: '', confirm_password: '', logout_all: false };
			}, 2000);
		} catch (err) {
			changePasswordError = err.message || 'Failed to change password';
		} finally {
			changePasswordLoading = false;
		}
	}

	function cancelChangePassword() {
		showChangePassword = false;
		changePasswordError = '';
		changePasswordData = { current_password: '', new_password: '', confirm_password: '', logout_all: false };
	}

	function copyToClipboard(text) {
		navigator.clipboard.writeText(text).then(() => {
			// Could show a toast notification here
		}).catch(() => {
			// Fallback for older browsers
			const textArea = document.createElement('textarea');
			textArea.value = text;
			document.body.appendChild(textArea);
			textArea.select();
			document.execCommand('copy');
			document.body.removeChild(textArea);
		});
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

</script>

<div class="max-w-4xl mx-auto space-y-6">
	<!-- Page Header -->
	<div class="mb-6">
		<h1 class="text-3xl font-bold flex items-center gap-3" style="color: var(--ds-text);">
			<Shield class="h-8 w-8" style="color: var(--ds-interactive);" />
			Security
		</h1>
		<p class="mt-2" style="color: var(--ds-text-subtle);">Manage your account security, authentication methods, and API access.</p>
	</div>

	<!-- Security Credentials -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<SectionHeader title="Security Credentials" subtitle="Manage your authentication methods" class="mb-6">
			{#snippet actions()}
				<Button
					variant="primary"
					onclick={() => showAddCredential = true}
					icon={Plus}
					size="medium"
					keyboardHint="A"
				>
					Add Credential
				</Button>
			{/snippet}
		</SectionHeader>

		<!-- Credentials List -->
		<div class="space-y-3">
			{#each credentials as credential}
				<div class="flex items-center justify-between p-4 border rounded hover-bg" style="border-color: var(--ds-border);">
					<div class="flex items-center space-x-3">
						<svelte:component this={getCredentialIcon(credential.credential_type)} class="h-6 w-6" style="color: var(--ds-icon-subtle);" />
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
						Remove
					</Button>
				</div>
			{:else}
				<div class="text-center py-12">
					<Shield class="h-12 w-12 mx-auto mb-4" style="color: var(--ds-icon-subtlest);" />
					<h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">No security credentials</h3>
					<p class="text-sm" style="color: var(--ds-text-subtle);">Add a security key or authenticator app to secure your account.</p>
				</div>
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
				<Button variant="link" onclick={() => showChangePassword = true}>
					Change Password
				</Button>
			</div>
		</div>
	</div>

	<!-- App Tokens -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<SectionHeader title="API Tokens" subtitle="Create tokens to access your account programmatically" class="mb-6">
			{#snippet actions()}
				<Button
					variant="primary"
					onclick={() => showAddToken = true}
					icon={Plus}
					size="medium"
					keyboardHint="A"
				>
					Create Token
				</Button>
			{/snippet}
		</SectionHeader>

		<!-- Show New Token -->
		{#if showNewToken}
			<div class="p-4 rounded mb-6" style="background-color: var(--ds-background-success-subtle); border: 1px solid var(--ds-border-success);">
				<h3 class="text-lg font-medium mb-2" style="color: var(--ds-text-success);">Token Created Successfully</h3>
				<p class="text-sm mb-4" style="color: var(--ds-text);">
					Copy this token now - you won't be able to see it again!
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
						Copy
					</Button>
				</div>
				<div class="mt-3">
					<Button
						variant="default"
						size="small"
						onclick={() => { showNewToken = false; newTokenValue = ''; }}
					>
						Done
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
						Revoke
					</Button>
				</div>
			{:else}
				<div class="text-center py-12">
					<Code class="h-12 w-12 mx-auto mb-4" style="color: var(--ds-icon-subtlest);" />
					<h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">No API tokens</h3>
					<p class="text-sm" style="color: var(--ds-text-subtle);">Generate your first API token to access your account programmatically.</p>
				</div>
			{/each}
		</div>
	</div>
</div>

<!-- Confirm Dialog -->
<ConfirmDialog
	bind:show={showConfirmDialog}
	title={confirmDialogConfig.title}
	message={confirmDialogConfig.message}
	variant="danger"
	icon={Trash2}
	confirmText="Delete"
	onconfirm={handleConfirm}
	oncancel={handleCancel}
/>

<!-- Change Password Modal -->
<Modal isOpen={showChangePassword} onclose={cancelChangePassword} maxWidth="max-w-md">
	<ModalHeader title="Change Password" onClose={cancelChangePassword} />

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
					<Label for="current-password" color="default" class="mb-1">Current Password</Label>
					<input
						id="current-password"
						type="password"
						bind:value={changePasswordData.current_password}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder="Enter current password"
					/>
				</div>

				<div>
					<Label for="new-password" color="default" class="mb-1">New Password</Label>
					<input
						id="new-password"
						type="password"
						bind:value={changePasswordData.new_password}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder="Enter new password (min 8 characters)"
					/>
				</div>

				<div>
					<Label for="confirm-password" color="default" class="mb-1">Confirm New Password</Label>
					<input
						id="confirm-password"
						type="password"
						bind:value={changePasswordData.confirm_password}
						class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
						style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
						placeholder="Confirm new password"
					/>
				</div>

				<div class="flex items-center gap-2">
					<input
						id="logout-all"
						type="checkbox"
						bind:checked={changePasswordData.logout_all}
						class="h-4 w-4 rounded"
						style="accent-color: var(--ds-interactive);"
					/>
					<label for="logout-all" class="text-sm" style="color: var(--ds-text-subtle);">
						Log out of all other sessions
					</label>
				</div>
			</div>
		{/if}
	</div>

	{#if !changePasswordSuccess}
		<DialogFooter
			cancelLabel="Cancel"
			confirmLabel="Change Password"
			onCancel={cancelChangePassword}
			onConfirm={handleChangePassword}
			confirmDisabled={changePasswordLoading || !changePasswordData.current_password || !changePasswordData.new_password || !changePasswordData.confirm_password}
			loading={changePasswordLoading}
		/>
	{/if}
</Modal>

<!-- Add Credential Modal -->
<Modal isOpen={showAddCredential} onclose={resetCredentialForm} maxWidth="max-w-lg">
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
							bind:group={credentialType}
							value="fido"
							class="mr-2"
						/>
						<Key class="h-4 w-4 mr-2" />
						<span style="color: var(--ds-text);">Security Key (FIDO2)</span>
					</label>
					<label class="flex items-center cursor-pointer">
						<input
							type="radio"
							bind:group={credentialType}
							value="ssh"
							class="mr-2"
						/>
						<Terminal class="h-4 w-4 mr-2" />
						<span style="color: var(--ds-text);">SSH Key</span>
					</label>
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
					bind:value={newCredentialName}
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
						bind:value={newSSHPublicKey}
						placeholder="ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAA... or ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAA..."
						rows={4}
						class="font-mono text-sm"
					/>
					<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Generate with: ssh-keygen -t ed25519 -C "your@email.com"</p>
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
					{enrollingFIDO ? 'Registering...' : 'Register Security Key'}
				{:else}
					{loading ? 'Adding...' : 'Add SSH Key'}
				{/if}
			</Button>
			<Button
				variant="default"
				onclick={resetCredentialForm}
				keyboardHint="Esc"
			>
				Cancel
			</Button>
		</div>
	</div>
</Modal>

<!-- Create Token Modal -->
<Modal isOpen={showAddToken} onclose={resetTokenForm} maxWidth="max-w-md">
	<div class="p-6">
		<h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
			Create New API Token
		</h3>

		<div class="space-y-4">
			<div>
				<Label for="token-name" color="default" class="mb-1">Token Name</Label>
				<input
					id="token-name"
					type="text"
					bind:value={newTokenName}
					placeholder="e.g., Mobile App, CI/CD Pipeline"
					class="w-full px-3 py-2 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
					style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
				/>
			</div>

			<div>
				<Label for="token-expiry" color="default" class="mb-1">Expiration (Optional)</Label>
				<input
					id="token-expiry"
					type="date"
					bind:value={newTokenExpiry}
					min={new Date().toISOString().split('T')[0]}
					class="w-full px-3 py-2 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
					style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
				/>
				<p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Leave empty for tokens that never expire</p>
			</div>
		</div>

		<div class="mt-6 flex gap-3">
			<Button
				variant="primary"
				onclick={createApiToken}
				disabled={!newTokenName.trim() || creatingToken}
				keyboardHint="⏎"
			>
				{creatingToken ? 'Creating...' : 'Create Token'}
			</Button>
			<Button
				variant="default"
				onclick={resetTokenForm}
				keyboardHint="Esc"
			>
				Cancel
			</Button>
		</div>
	</div>
</Modal>