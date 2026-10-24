<script>
	import { currentRoute, navigate } from '../router.js';
	import { authStore } from '../stores/auth.svelte.js';
	import { securityStore } from '../stores/securityStore.svelte.js';
	import { t } from '../stores/i18n.svelte.js';
	import User from '@lucide/svelte/icons/user';
	import Shield from '@lucide/svelte/icons/shield';
	import Key from '@lucide/svelte/icons/key';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import Plus from '@lucide/svelte/icons/plus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Code from '@lucide/svelte/icons/code';
	import Copy from '@lucide/svelte/icons/copy';
	import Terminal from '@lucide/svelte/icons/terminal';
	import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
	import X from '@lucide/svelte/icons/x';
	import Button from '../components/Button.svelte';
	import Input from '../components/Input.svelte';
	import TextField from '../components/TextField.svelte';
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
	import { formatDate } from '../utils/dateFormatter.js';
	import { formatAuthenticatedInstant } from '../utils/authenticatedDateFormatter.js';
	import { errorToast, successToast } from '../stores/toasts.svelte.js';
	import Checkbox from '../components/Checkbox.svelte';
	import Radio from '../components/Radio.svelte';
	import DescriptionText from '../components/DescriptionText.svelte';
	import {
		isWebAuthnSupported,
		prepareCredentialCreationOptions,
		processCredentialCreationResponse
	} from '../utils/webauthn-utils.js';
	import { toHotkeyString } from '../utils/keyboardShortcuts.js';

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
	let enrollmentOnly = $derived(securityStore.enrollmentOnly);
	let showChangePassword = $derived(securityStore.showChangePassword);
	let changePasswordData = $derived(securityStore.changePasswordData);
	let changePasswordLoading = $derived(securityStore.changePasswordLoading);
	let changePasswordError = $derived(securityStore.changePasswordError);
	let changePasswordSuccess = $derived(securityStore.changePasswordSuccess);

	// Derived from auth store
	let currentUserId = $derived(authStore.currentUser?.id);
	let isSystemAdmin = $derived(authStore.currentUser?.is_system_admin === true);

	// The grantable scopes come from the server (GET /api-tokens/scope-catalog,
	// backed by auth.ScopeCatalog) rather than a list maintained here. A
	// hand-written copy silently dropped whole resources as scopes were added —
	// time:*, tests:*, actions:* and even mcp:access were unreachable from this
	// picker — so the catalog is now the only source of truth.
	let scopeCatalog = $derived(securityStore.scopeCatalog);

	// Group the flat catalog into one block per resource, preserving the
	// server's ordering, so any action name (read/write/delete/access) renders
	// without this page needing to know the column layout in advance.
	function groupScopes(entries) {
		const groups = [];
		const byResource = new Map();
		for (const info of entries) {
			let group = byResource.get(info.resource);
			if (!group) {
				group = { resource: info.resource, label: info.resource_label, scopes: [] };
				byResource.set(info.resource, group);
				groups.push(group);
			}
			group.scopes.push(info);
		}
		return groups;
	}

	let scopeGroups = $derived(groupScopes(scopeCatalog.filter((s) => !s.admin)));
	let adminScopeGroups = $derived(groupScopes(scopeCatalog.filter((s) => s.admin)));

	// Presets derived from the catalog. "Agent default" is the set an MCP or
	// `ws` CLI token receives when minted without an explicit scope list, so a
	// hand-made token can match one without ticking 27 boxes.
	let agentDefaultPreset = $derived(
		scopeCatalog.filter((s) => s.agent_default).map((s) => s.scope)
	);
	let readOnlyPreset = $derived(
		scopeCatalog.filter((s) => !s.admin && s.action === 'read').map((s) => s.scope)
	);

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

	// Initialize in a dedicated enrollment-only mode when login issued a
	// restricted first-passkey session. This must happen before loading normal
	// Security-page resources, which the server intentionally denies.
	$effect(() => {
		const enrollmentRequested = $currentRoute.query?.enroll === 'passkey';
		if (enrollmentRequested) {
			securityStore.checkEnrollmentRequired('passkey');
		}
		if (currentUserId) {
			securityStore.setCurrentUserId(currentUserId, { enrollmentOnly: enrollmentRequested });
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
				navigate('/security');
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

	function getCredentialName(credential) {
		return credential.name || credential.credential_name || '';
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

<div class="max-w-4xl mx-auto space-y-6" data-testid="security-page">
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
				{#if !enrollmentOnly}
					<button
						type="button"
						onclick={dismissEnrollmentBanner}
						class="p-1 rounded hover:bg-black/10"
						style="color: var(--ds-text-warning-inverse);"
					>
						<X class="w-5 h-5" />
					</button>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Security Credentials -->
	<div class="shadow rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<SectionHeader title={t('security.credentials')} subtitle={t('security.credentialsSubtitle')} class="mb-6">
			{#snippet actions()}
				{#if !enrollmentOnly}
					<Button
						variant="primary"
						onclick={() => securityStore.showAddCredential = true}
						icon={Plus}
						size="medium"
						keyboardHint="A"
						dataTestid="security-add-credential"
						hotkeyConfig={{ key: toHotkeyString('security', 'addCredential'), guard: () => !showAddCredential && !showAddToken && !showChangePassword }}
					>
						{t('common.add')}
					</Button>
				{/if}
			{/snippet}
		</SectionHeader>

		<!-- Credentials List -->
		<div class="space-y-3">
			{#each credentials as credential}
				{@const CredIcon = getCredentialIcon(credential.credential_type)}
				<div class="flex items-center justify-between p-4 border rounded hover-bg" style="border-color: var(--ds-border);" data-testid="security-credential-row">
					<div class="flex items-center space-x-3">
						<CredIcon class="h-6 w-6" style="color: var(--ds-icon-subtle);" />
						<div>
							<div class="font-medium" style="color: var(--ds-text);" data-testid="security-credential-name">{getCredentialName(credential)}</div>
							<div class="text-sm" style="color: var(--ds-text-subtle);">
								{getCredentialTypeName(credential.credential_type)} • Added {formatAuthenticatedInstant(credential.created_at, { year: 'numeric', month: 'short', day: 'numeric' }) || '-'}
							</div>
						</div>
					</div>
					<Button
						variant="default"
						size="small"
						icon={Trash2}
						onclick={() => confirmRemoveCredential(credential.id, getCredentialName(credential))}
						dataTestid="security-credential-remove"
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

	{#if !enrollmentOnly}
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
					hotkeyConfig={{ key: toHotkeyString('security', 'createToken'), guard: () => !showAddCredential && !showAddToken && !showChangePassword }}
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
					<Input
						type="text"
						value={newTokenValue}
						readonly
						class="flex-1 font-mono border-[var(--ds-border-success)]"
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
								Created {formatAuthenticatedInstant(token.created_at, { year: 'numeric', month: 'short', day: 'numeric' }) || '-'} • Expires {formatAuthenticatedInstant(token.expires_at, { year: 'numeric', month: 'short', day: 'numeric' }) || 'Never expires'}
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
	{/if}
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
					<TextField
						label={t('auth.currentPassword')}
						labelColor="default"
						labelClass="mb-1"
						id="current-password"
						type="password"
						value={changePasswordData.current_password}
						oninput={(e) => setChangePasswordData('current_password', /** @type {HTMLInputElement} */ (e.target).value)}
						placeholder={t('placeholders.enterPassword')}
					/>
				</div>

				<div>
					<TextField
						label={t('auth.newPassword')}
						labelColor="default"
						labelClass="mb-1"
						id="new-password"
						type="password"
						value={changePasswordData.new_password}
						oninput={(e) => setChangePasswordData('new_password', /** @type {HTMLInputElement} */ (e.target).value)}
						placeholder={t('placeholders.enterNewPassword')}
					/>
				</div>

				<div>
					<TextField
						label={t('auth.confirmPassword')}
						labelColor="default"
						labelClass="mb-1"
						id="confirm-password"
						type="password"
						value={changePasswordData.confirm_password}
						oninput={(e) => setChangePasswordData('confirm_password', /** @type {HTMLInputElement} */ (e.target).value)}
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
<Modal isOpen={showAddCredential} onclose={() => securityStore.resetCredentialForm()} maxWidth="max-w-lg" dataTestid="security-credential-modal">
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
						<Radio
							checked={credentialType === 'fido'}
							onchange={() => setCredentialType('fido')}
							class="mr-2"
						/>
						<Key class="h-4 w-4 mr-2" />
						<span style="color: var(--ds-text);">Security Key (FIDO2)</span>
					</label>
					{#if sshAvailable && !enrollmentOnly}
					<label class="flex items-center cursor-pointer">
						<Radio
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
				<TextField
					label={credentialType === 'fido' ? 'Security Key Name' : 'SSH Key Name'}
					labelColor="default"
					labelClass="mb-1"
					id="credential-name"
					value={newCredentialName}
					oninput={(e) => setNewCredentialName(/** @type {HTMLInputElement} */ (e.target).value)}
					placeholder={credentialType === 'fido' ? 'e.g., YubiKey, iPhone Touch ID' : 'e.g., MacBook Pro, CI Server'}
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
				dataTestid="security-register-credential"
				keyboardHint="⏎"
			>
				{#if credentialType === 'fido'}
					{enrollingFIDO ? t('common.processing') : 'Register Security Key'}
				{:else}
					{loading ? t('common.processing') : 'Add SSH Key'}
				{/if}
			</Button>
			{#if !enrollmentOnly}
				<Button
					variant="default"
					onclick={() => securityStore.resetCredentialForm()}
					keyboardHint="Esc"
				>
					{t('common.cancel')}
				</Button>
			{/if}
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
				<TextField
					label={t('security.tokenName')}
					labelColor="default"
					labelClass="mb-1"
					id="token-name"
					value={newTokenName}
					oninput={(e) => setNewTokenName(/** @type {HTMLInputElement} */ (e.target).value)}
					placeholder="e.g., Mobile App, CI/CD Pipeline"
				/>
			</div>

			<div>
				<div class="flex items-center justify-between mb-2">
					<Label color="default" class="mb-0">Permissions</Label>
					<div class="flex gap-2">
						<Button variant="link" size="small" dataTestid="token-scope-preset-agent-default" onclick={() => applyScopePreset(agentDefaultPreset)}>Agent default</Button>
						<Button variant="link" size="small" dataTestid="token-scope-preset-read-only" onclick={() => applyScopePreset(readOnlyPreset)}>Read-only</Button>
						<Button variant="link" size="small" dataTestid="token-scope-preset-clear" onclick={clearScopes}>Clear</Button>
					</div>
				</div>
				<DescriptionText>
					Scopes limit what this token can do. The token also inherits your account's workspace permissions.
				</DescriptionText>

				<div class="mt-3 rounded border overflow-hidden" style="border-color: var(--ds-border);" data-testid="token-scope-picker">
					{#each scopeGroups as group (group.resource)}
						<div class="border-t first:border-t-0 px-3 py-2" style="border-color: var(--ds-border);">
							<div class="text-sm font-medium" style="color: var(--ds-text);">{group.label}</div>
							<div class="mt-1 flex flex-wrap gap-x-4 gap-y-1">
								{#each group.scopes as info (info.scope)}
									<Checkbox
										checked={isScopeSelected(info.scope)}
										onchange={(checked) => toggleScope(info.scope, checked)}
										label={info.action}
										size="small"
										dataTestid={`token-scope-${info.scope}`}
									/>
								{/each}
							</div>
						</div>
					{/each}
				</div>

				{#if isSystemAdmin}
					<div class="mt-4">
						<Label color="default" class="mb-2">Admin permissions</Label>
						<DescriptionText>Require the system admin role. Use with care.</DescriptionText>
						<div class="mt-2 rounded border overflow-hidden" style="border-color: var(--ds-border);" data-testid="token-admin-scope-picker">
							{#each adminScopeGroups as group (group.resource)}
								<div class="border-t first:border-t-0 px-3 py-2" style="border-color: var(--ds-border);">
									<div class="text-sm font-medium" style="color: var(--ds-text);">{group.label}</div>
									<div class="mt-1 flex flex-wrap gap-x-4 gap-y-1">
										{#each group.scopes as info (info.scope)}
											<Checkbox
												checked={isScopeSelected(info.scope)}
												onchange={(checked) => toggleScope(info.scope, checked)}
												label={info.action}
												size="small"
												dataTestid={`token-scope-${info.scope}`}
											/>
										{/each}
									</div>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>

			<div>
				<Label for="token-expiry" color="default" class="mb-1">Last valid date (optional)</Label>
				<Input
					id="token-expiry"
					type="date"
					value={newTokenExpiry}
					oninput={(e) => setNewTokenExpiry(/** @type {HTMLInputElement} */ (e.target).value)}
					min={formatDate(new Date())}
				/>
				<DescriptionText>The token remains valid through this date in your configured timezone. Leave empty for no expiration.</DescriptionText>
			</div>
		</div>

		<div class="mt-6 flex gap-3">
			<Button
				variant="primary"
				onclick={createApiToken}
				disabled={!newTokenName.trim() || newTokenScopes.length === 0 || creatingToken}
				dataTestid="create-token-submit"
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
