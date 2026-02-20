<script>
	import { onMount } from 'svelte';
	import { api, getCalendarFeedToken, createCalendarFeedToken, revokeCalendarFeedToken } from '../api.js';
	import { authStore, attachmentStatus } from '../stores';
	import { User, Shield, Key, Smartphone, Plus, Trash2, Calendar, CheckCircle, PlayCircle, Code, Copy, Eye, EyeOff, Camera, Upload, Globe, CalendarDays, RefreshCw, Link2, ExternalLink, GitBranch } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import Tabs from '../components/Tabs.svelte';
	import Spinner from '../components/Spinner.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import BasePicker from '../pickers/BasePicker.svelte';
	import ConnectedAccountsTab from '../settings/ConnectedAccountsTab.svelte';
	import { formatDate } from '../utils/dateFormatter.js';
	import { t, i18n, SUPPORTED_LOCALES } from '../stores/i18n.svelte.js';
	import {
		isWebAuthnSupported,
		registerCredential,
		getWebAuthnErrorMessage,
		base64urlToArrayBuffer,
		arrayBufferToBase64url
	} from '../utils/webauthn-utils.js';

	let user = null;
	let credentials = [];
	let appTokens = [];
	let loading = false;
	let error = '';
	let showAddCredential = false;
	let enrollingFIDO = false;
	let newCredentialName = '';
	let testingLogin = false;
	let loginTestResult = '';
	let showAddToken = false;
	let creatingToken = false;
	let newTokenName = '';
	let newTokenScopes = [];
	let newTokenExpiry = '';
	let showNewToken = false;
	let newTokenValue = '';

	// Avatar management state
	let showAvatarUpload = false;
	let uploadingAvatar = false;

	// Regional settings state
	let selectedTimezone = 'UTC';
	let selectedLanguage = 'en';
	let savingRegionalSettings = false;
	let regionalSettingsSaved = false;

	// Calendar feed state
	let calendarFeedInfo = null;
	let loadingCalendarFeed = false;
	let calendarFeedError = '';
	let generatingFeed = false;
	let revokingFeed = false;
	let showFullFeedUrl = false;
	let feedUrlCopied = false;

	// Tab state
	let activeTab = 'avatar'; // Default to avatar tab

	// Use current user ID from auth store
	$: currentUserId = authStore.currentUser?.id;

	// Configure tabs based on whether attachments are enabled
	$: tabs = [
		...(attachmentStatus.enabled ? [{
			id: 'avatar',
			label: t('users.avatar'),
			icon: Camera
		}] : []),
		{
			id: 'regional-settings',
			label: t('users.regionalSettings'),
			icon: Globe
		},
		{
			id: 'connected-accounts',
			label: t('users.connectedAccounts'),
			icon: GitBranch
		},
		{
			id: 'calendar-integration',
			label: t('users.calendarIntegration'),
			icon: CalendarDays
		}
	];

	// Set initial active tab (avatar if attachments enabled, otherwise regional-settings)
	$: if (tabs.length > 0 && !tabs.find(t => t.id === activeTab)) {
		activeTab = tabs[0].id;
	}

	onMount(() => {
		if (currentUserId) {
			loadUserProfile();
			loadCredentials();
			loadAppTokens();
		}
	});

	// Watch for currentUserId changes and load data when available
	$: if (currentUserId && !user) {
		loadUserProfile();
		loadCredentials();
		loadAppTokens();
	}

	async function loadUserProfile() {
		if (!currentUserId) return;
		try {
			user = await api.getUser(currentUserId);
			// Populate regional settings when user data loads
			if (user) {
				selectedTimezone = user.timezone || 'UTC';
				selectedLanguage = user.language || 'en';
			}
		} catch (err) {
			error = t('dialogs.alerts.failedToLoad', { error: 'user profile' });
		}
	}

	async function loadCredentials() {
		if (!currentUserId) return;
		try {
			credentials = await api.getUserCredentials(currentUserId);
		} catch (err) {
			error = t('dialogs.alerts.failedToLoad', { error: 'credentials' });
		}
	}

	async function handleAvatarUpload(files) {
		if (!currentUserId || !files || files.length === 0) return;

		const file = files[0];
		if (!file.type.startsWith('image/')) {
			error = t('dialogs.alerts.pleaseSelectImage');
			return;
		}

		uploadingAvatar = true;
		try {
			const formData = new FormData();
			formData.append('file', file);
			formData.append('item_id', '0'); // Use 0 for avatar uploads
			formData.append('category', 'avatar');

			const uploadResult = await api.attachments.upload(formData);
			
			if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
				// Use the avatar_url directly from the upload response
				await api.updateUserAvatar(currentUserId, uploadResult.avatar_url);
				
				// Reload user profile to show new avatar
				await loadUserProfile();
				showAvatarUpload = false;
			}
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToUpload', { error: 'avatar' });
		} finally {
			uploadingAvatar = false;
		}
	}

	async function removeAvatar() {
		if (!currentUserId) return;
		if (!confirm(t('dialogs.confirmations.removeAvatar'))) return;

		try {
			await api.updateUserAvatar(currentUserId, null);
			await loadUserProfile();
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToDelete', { error: 'avatar' });
		}
	}

	async function loadAppTokens() {
		if (!currentUserId) return;
		try {
			appTokens = await api.getUserAppTokens(currentUserId);
		} catch (err) {
			error = t('dialogs.alerts.failedToLoad', { error: 'app tokens' });
		}
	}

	async function enrollFIDOKey() {
		if (!newCredentialName.trim()) {
			error = t('security.enterSecurityKeyName');
			return;
		}

		// Check if WebAuthn is supported
		if (!isWebAuthnSupported()) {
			error = t('security.webAuthnNotSupported');
			return;
		}

		enrollingFIDO = true;
		error = '';

		try {
			// Start FIDO registration
			const registrationResponse = await api.startFIDORegistration(currentUserId, newCredentialName);

			// Extract session ID for new API format
			const sessionId = registrationResponse.sessionId;

			// Get the publicKey options
			const publicKeyOptions = registrationResponse.publicKey || registrationResponse.options || registrationResponse;

			if (!publicKeyOptions || !publicKeyOptions.challenge) {
				throw new Error(t('security.invalidRegistrationChallenge'));
			}

			// Use the utility function to create credential
			const credentialResponse = await registerCredential(publicKeyOptions);

			// Complete registration with server (include sessionId if present)
			const registrationData = sessionId
				? { sessionId, credentialName: newCredentialName, response: credentialResponse }
				: { ...credentialResponse, credentialName: newCredentialName };

			await api.completeFIDORegistration(currentUserId, registrationData);

			// Reload credentials and reset form
			await loadCredentials();
			newCredentialName = '';
			showAddCredential = false;

		} catch (err) {
			error = getWebAuthnErrorMessage(err);
		} finally {
			enrollingFIDO = false;
		}
	}

	async function removeCredential(credentialId, credentialName) {
		if (!confirm(t('dialogs.confirmations.deleteItem', { name: credentialName }))) return;

		try {
			await api.removeUserCredential(currentUserId, credentialId);
			await loadCredentials();
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToDelete', { error: 'credential' });
		}
	}

	async function testFIDOLogin() {
		testingLogin = true;
		loginTestResult = '';
		error = '';

		try {
			// Check if WebAuthn is supported
			if (!window.PublicKeyCredential) {
				throw new Error(t('security.webAuthnNotSupported'));
			}

			// Check if user has any FIDO credentials
			const fidoCredentials = credentials.filter(c => c.credential_type === 'fido' && c.is_active);
			if (fidoCredentials.length === 0) {
				throw new Error(t('security.noActiveFidoCredentials'));
			}

			// Mock authentication challenge (in production this would come from your auth endpoint)
			const mockChallenge = crypto.getRandomValues(new Uint8Array(32));
			
			// Create assertion options
			const assertionOptions = {
				challenge: mockChallenge,
				allowCredentials: fidoCredentials.map(cred => {
					// Parse credential data to get the credential ID
					try {
						const credData = JSON.parse(cred.credential_data);
						return {
							type: 'public-key',
							id: base64urlToArrayBuffer(credData.rawId)
						};
					} catch (e) {
						console.warn('Could not parse credential data:', e);
						return null;
					}
				}).filter(Boolean),
				userVerification: 'preferred',
				timeout: 60000
			};

			// Request assertion - this will prompt for security key
			const assertion = await navigator.credentials.get({
				publicKey: assertionOptions
			});

			if (assertion) {
				loginTestResult = 'success';
			} else {
				throw new Error(t('security.authenticationFailed'));
			}

		} catch (err) {
			loginTestResult = 'error';
			if (err.name === 'NotAllowedError') {
				error = t('security.authenticationCancelled');
			} else {
				error = err.message || t('security.failedToTestFidoLogin');
			}
		} finally {
			testingLogin = false;
		}
	}

	async function createAppToken() {
		if (!newTokenName.trim()) {
			error = t('security.enterTokenName');
			return;
		}

		creatingToken = true;
		error = '';

		try {
			const tokenData = {
				token_name: newTokenName,
				scopes: newTokenScopes,
				expires_at: newTokenExpiry || null
			};

			const response = await api.createAppToken(currentUserId, tokenData);
			
			// Show the new token value
			newTokenValue = response.token;
			showNewToken = true;
			
			// Reset form first
			resetTokenForm();
			
			// Then reload tokens to refresh the list
			await loadAppTokens();
			
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToCreate', { error: 'app token' });
		} finally {
			creatingToken = false;
		}
	}

	async function revokeAppToken(tokenId, tokenName) {
		if (!confirm(t('security.confirmRevokeToken', { name: tokenName }))) return;

		try {
			await api.revokeAppToken(currentUserId, tokenId);
			await loadAppTokens();
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToDelete', { error: 'token' });
		}
	}

	function resetTokenForm() {
		newTokenName = '';
		newTokenScopes = [];
		newTokenExpiry = '';
		showAddToken = false;
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
			default:
				return Shield;
		}
	}

	function getCredentialTypeName(type) {
		switch (type) {
			case 'fido':
				return t('security.securityKeyFido');
			case 'totp':
				return t('security.authenticatorAppTotp');
			default:
				return t('common.unknown');
		}
	}

	// Helper functions moved to webauthn-utils.js

	async function saveRegionalSettings() {
		if (!currentUserId || !user) return;

		savingRegionalSettings = true;
		error = '';
		regionalSettingsSaved = false;

		try {
			// Use dedicated endpoint that only updates regional settings
			await api.updateUserRegionalSettings(currentUserId, {
				timezone: selectedTimezone,
				language: selectedLanguage
			});

			// Switch UI locale to match saved preference
			await i18n.setLocale(selectedLanguage);

			await loadUserProfile();

			regionalSettingsSaved = true;
			setTimeout(() => {
				regionalSettingsSaved = false;
			}, 3000);
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToSave', { error: 'regional settings' });
		} finally {
			savingRegionalSettings = false;
		}
	}

	// Calendar feed functions
	async function loadCalendarFeedInfo() {
		loadingCalendarFeed = true;
		calendarFeedError = '';
		try {
			calendarFeedInfo = await getCalendarFeedToken();
		} catch (err) {
			if (err.message?.includes('disabled')) {
				calendarFeedError = t('users.calendarFeedsDisabled');
			} else {
				calendarFeedError = err.message || t('dialogs.alerts.failedToLoad', { error: 'calendar feed info' });
			}
		} finally {
			loadingCalendarFeed = false;
		}
	}

	async function generateCalendarFeed() {
		generatingFeed = true;
		calendarFeedError = '';
		try {
			const result = await createCalendarFeedToken();
			// Reload feed info to get complete data
			await loadCalendarFeedInfo();
			// Show the full URL since they just generated it
			showFullFeedUrl = true;
		} catch (err) {
			if (err.message?.includes('disabled')) {
				calendarFeedError = t('users.calendarFeedsDisabled');
			} else {
				calendarFeedError = err.message || t('dialogs.alerts.failedToCreate', { error: 'calendar feed' });
			}
		} finally {
			generatingFeed = false;
		}
	}

	async function revokeCalendarFeed() {
		if (!confirm(t('dialogs.confirmations.revokeCalendarFeed'))) {
			return;
		}

		revokingFeed = true;
		calendarFeedError = '';
		try {
			await revokeCalendarFeedToken();
			calendarFeedInfo = { has_token: false };
			showFullFeedUrl = false;
		} catch (err) {
			calendarFeedError = err.message || t('dialogs.alerts.failedToDelete', { error: 'calendar feed' });
		} finally {
			revokingFeed = false;
		}
	}

	function copyFeedUrl() {
		if (calendarFeedInfo?.feed?.feed_url) {
			navigator.clipboard.writeText(calendarFeedInfo.feed.feed_url).then(() => {
				feedUrlCopied = true;
				setTimeout(() => feedUrlCopied = false, 2000);
			}).catch(err => {
				// Fallback
				const textArea = document.createElement('textarea');
				textArea.value = calendarFeedInfo.feed.feed_url;
				document.body.appendChild(textArea);
				textArea.select();
				document.execCommand('copy');
				document.body.removeChild(textArea);
				feedUrlCopied = true;
				setTimeout(() => feedUrlCopied = false, 2000);
			});
		}
	}

	function getMaskedFeedUrl(url) {
		if (!url) return '';
		// Show first 40 chars and last 20 chars
		if (url.length <= 70) return url;
		return url.substring(0, 40) + '...' + url.substring(url.length - 20);
	}

	// Common timezones (IANA format)
	const commonTimezones = [
		{ value: 'UTC', label: 'UTC (Coordinated Universal Time)' },
		{ value: 'America/New_York', label: 'Eastern Time (US & Canada)' },
		{ value: 'America/Chicago', label: 'Central Time (US & Canada)' },
		{ value: 'America/Denver', label: 'Mountain Time (US & Canada)' },
		{ value: 'America/Los_Angeles', label: 'Pacific Time (US & Canada)' },
		{ value: 'America/Anchorage', label: 'Alaska Time' },
		{ value: 'Pacific/Honolulu', label: 'Hawaii Time' },
		{ value: 'Europe/London', label: 'London (GMT/BST)' },
		{ value: 'Europe/Paris', label: 'Paris (CET/CEST)' },
		{ value: 'Europe/Berlin', label: 'Berlin (CET/CEST)' },
		{ value: 'Europe/Rome', label: 'Rome (CET/CEST)' },
		{ value: 'Europe/Madrid', label: 'Madrid (CET/CEST)' },
		{ value: 'Asia/Tokyo', label: 'Tokyo (JST)' },
		{ value: 'Asia/Shanghai', label: 'Shanghai (CST)' },
		{ value: 'Asia/Hong_Kong', label: 'Hong Kong (HKT)' },
		{ value: 'Asia/Singapore', label: 'Singapore (SGT)' },
		{ value: 'Asia/Dubai', label: 'Dubai (GST)' },
		{ value: 'Asia/Kolkata', label: 'India (IST)' },
		{ value: 'Australia/Sydney', label: 'Sydney (AEDT/AEST)' },
		{ value: 'Australia/Melbourne', label: 'Melbourne (AEDT/AEST)' },
		{ value: 'Pacific/Auckland', label: 'Auckland (NZDT/NZST)' }
	];

	// Languages - only show those with UI translations
	const commonLanguages = SUPPORTED_LOCALES.map(loc => ({
		value: loc.code,
		label: loc.code === 'en' ? 'English' :
		       loc.code === 'de' ? 'Deutsch (German)' :
		       loc.code === 'es' ? 'Español (Spanish)' :
		       loc.code === 'ar' ? 'العربية (Arabic)' : loc.name
	}));
</script>

<div class="max-w-4xl mx-auto px-6 py-8 space-y-6">
	<!-- Page Header -->
	<PageHeader
		icon={User}
		title={t('users.profile')}
		subtitle={t('users.profileSubtitle')}
	/>

	<!-- Profile Information -->
	<div class="shadow rounded p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
		<h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('users.profileInformation')}</h2>
		{#if user}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">{t('users.fullName')}</span>
					<p class="mt-1 text-sm" style="color: var(--ds-text);">{user.full_name}</p>
				</div>
				<div>
					<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">{t('common.email')}</span>
					<p class="mt-1 text-sm" style="color: var(--ds-text);">{user.email}</p>
				</div>
				{#if user.requires_password_reset}
					<div>
						<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">{t('common.status')}</span>
						<span class="mt-1 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800">
							{t('users.passwordResetRequired')}
						</span>
					</div>
				{/if}
			</div>
		{:else}
			<div class="animate-pulse space-y-4">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<div class="h-4 rounded w-16 mb-2" style="background-color: var(--ds-background-neutral);"></div>
						<div class="h-4 rounded w-32" style="background-color: var(--ds-background-neutral);"></div>
					</div>
					<div>
						<div class="h-4 rounded w-12 mb-2" style="background-color: var(--ds-background-neutral);"></div>
						<div class="h-4 rounded w-48" style="background-color: var(--ds-background-neutral);"></div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<!-- Tabbed Settings -->
	<Tabs {tabs} bind:activeTab>
		<!-- Avatar Management Tab -->
		{#if activeTab === 'avatar' && attachmentStatus.enabled}
			<div class="flex items-center justify-between mb-6">
				<div>
					<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
						<Camera class="h-5 w-5" style="color: var(--ds-text-subtle);" />
						{t('users.profilePicture')}
					</h2>
					<p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.uploadAndManageAvatar')}</p>
				</div>
				<div class="flex items-center gap-2">
					{#if user?.avatar_url}
						<Button
							variant="default"
							onclick={removeAvatar}
							icon={Trash2}
							size="medium"
						>
							{t('common.remove')}
						</Button>
					{/if}
					<Button
						variant="primary"
						onclick={() => showAvatarUpload = !showAvatarUpload}
						icon={Upload}
						size="medium"
					>
						{user?.avatar_url ? t('users.changeAvatar') : t('users.uploadAvatar')}
					</Button>
				</div>
			</div>

			<!-- Current Avatar Display -->
			<div class="flex items-center gap-6 mb-6">
				<div class="relative">
					{#if user?.avatar_url}
						<img class="h-20 w-20 rounded-full border-2" style="border-color: var(--ds-border);" src={user.avatar_url} alt="Current avatar" />
					{:else}
						<div class="h-20 w-20 rounded-full flex items-center justify-center border-2" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
							<User class="h-10 w-10" style="color: var(--ds-icon);" />
						</div>
					{/if}
				</div>
				<div>
					<h3 class="font-medium" style="color: var(--ds-text);">{t('users.currentProfilePicture')}</h3>
					<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
						{user?.avatar_url ? t('users.customAvatarActive') : t('users.usingDefaultAvatar')}
					</p>
					<p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
						{t('users.avatarRecommendation')}
					</p>
				</div>
			</div>

			<!-- Avatar Upload Interface -->
			{#if showAvatarUpload}
				<div class="border rounded p-4" style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border);">
					<h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('users.uploadNewAvatar')}</h3>

					<div class="mb-4">
						<input
							type="file"
							accept="image/*"
							onchange={(e) => handleAvatarUpload(e.target.files)}
							disabled={uploadingAvatar}
							class="block w-full text-sm file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-600 file:text-white hover:file:bg-blue-700 disabled:opacity-50"
							style="color: var(--ds-text-subtle);"
						/>
						<p class="text-xs mt-2" style="color: var(--ds-text-subtlest);">
							{t('users.avatarFileHint')}
						</p>
					</div>

					{#if uploadingAvatar}
						<div class="mb-4">
							<div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
								<Spinner size="sm" />
								{t('users.uploadingAvatar')}
							</div>
						</div>
					{/if}

					<div class="flex justify-end gap-2">
						<Button
							variant="default"
							onclick={() => showAvatarUpload = false}
							size="small"
							disabled={uploadingAvatar}
						>
							{t('common.cancel')}
						</Button>
					</div>
				</div>
			{/if}
		{/if}

		<!-- Regional Settings Tab -->
		{#if activeTab === 'regional-settings'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<Globe class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					{t('users.regionalSettings')}
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.regionalSettingsDesc')}</p>
			</div>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
			<!-- Timezone Selection -->
			<div>
				<label for="timezone" class="block text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">
					{t('users.timezone')}
				</label>
				<BasePicker
					bind:value={selectedTimezone}
					items={commonTimezones}
					placeholder={t('users.timezone')}
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
				<p class="text-xs mt-2" style="color: var(--ds-text-subtlest);">
					{t('users.timezoneHint')}
				</p>
			</div>

			<!-- Language Selection -->
			<div>
				<label for="language" class="block text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">
					{t('users.language')}
				</label>
				<BasePicker
					bind:value={selectedLanguage}
					items={commonLanguages}
					placeholder={t('users.language')}
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
				<p class="text-xs mt-2" style="color: var(--ds-text-subtlest);">
					{t('users.languageHint')}
				</p>
			</div>
		</div>

		<!-- Save Button and Success Message -->
		<div class="mt-6 flex items-center gap-4">
			<Button
				variant="primary"
				onclick={saveRegionalSettings}
				disabled={!user || savingRegionalSettings}
				size="medium"
			>
				{savingRegionalSettings ? t('common.saving') : t('users.saveSettings')}
			</Button>

			{#if regionalSettingsSaved}
				<AlertBox variant="success" message={t('users.settingsSaved')} />
			{/if}
		</div>
		{/if}

		<!-- Connected Accounts Tab -->
		{#if activeTab === 'connected-accounts'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<GitBranch class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					{t('users.connectedAccounts')}
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					{t('users.connectedAccountsDesc')}
				</p>
			</div>

			<ConnectedAccountsTab />
		{/if}

		<!-- Calendar Integration Tab -->
		{#if activeTab === 'calendar-integration'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<CalendarDays class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					{t('users.calendarIntegration')}
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					{t('users.calendarIntegrationDesc')}
				</p>
			</div>

			{#if calendarFeedError}
				<AlertBox message={calendarFeedError} />
			{/if}

			{#if loadingCalendarFeed}
				<div class="flex items-center justify-center py-8">
					<Spinner size="md" />
				</div>
			{:else if !calendarFeedInfo}
				<!-- Load feed info when tab becomes active -->
				<div class="py-4">
					<Button variant="default" onclick={loadCalendarFeedInfo}>
						{t('users.loadCalendarFeedSettings')}
					</Button>
				</div>
			{:else if !calendarFeedInfo.has_token}
				<!-- No feed token yet -->
				<div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
					<div class="flex items-start gap-4">
						<div class="p-3 rounded-lg" style="background-color: var(--ds-background-neutral);">
							<Link2 class="w-6 h-6" style="color: var(--ds-icon);" />
						</div>
						<div class="flex-1">
							<h3 class="text-base font-medium" style="color: var(--ds-text);">{t('users.enableCalendarSubscription')}</h3>
							<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
								{t('users.calendarSubscriptionDesc')}
							</p>
							<div class="mt-4">
								<Button
									variant="primary"
									onclick={generateCalendarFeed}
									disabled={generatingFeed}
									icon={CalendarDays}
								>
									{generatingFeed ? t('common.generating') : t('users.generateCalendarFeedUrl')}
								</Button>
							</div>
						</div>
					</div>
				</div>
			{:else}
				<!-- Has feed token -->
				<div class="space-y-6">
					<!-- Feed URL Display -->
					<div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
						<div class="flex items-center justify-between mb-4">
							<h3 class="text-base font-medium" style="color: var(--ds-text);">{t('users.yourCalendarFeedUrl')}</h3>
							<div class="flex items-center gap-2">
								<button
									class="text-sm px-2 py-1 rounded hover-bg"
									style="color: var(--ds-link);"
									onclick={() => showFullFeedUrl = !showFullFeedUrl}
								>
									{#if showFullFeedUrl}
										<EyeOff class="w-4 h-4 inline mr-1" />
										{t('common.hide')}
									{:else}
										<Eye class="w-4 h-4 inline mr-1" />
										{t('users.showFullUrl')}
									{/if}
								</button>
							</div>
						</div>

						<div class="flex items-center gap-2">
							<input
								type="text"
								readonly
								value={showFullFeedUrl ? calendarFeedInfo.feed?.feed_url : getMaskedFeedUrl(calendarFeedInfo.feed?.feed_url)}
								class="flex-1 px-3 py-2 text-sm border rounded-md font-mono"
								style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
							/>
							<Button
								variant="default"
								onclick={copyFeedUrl}
								icon={Copy}
								size="small"
							>
								{feedUrlCopied ? t('toast.copied') : t('common.copy')}
							</Button>
						</div>

						<p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
							{t('users.calendarFeedWarning')}
						</p>

						{#if calendarFeedInfo.feed?.last_accessed_at}
							<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
								{t('users.lastSynced')}: {formatDate(calendarFeedInfo.feed.last_accessed_at, { relative: true })}
							</p>
						{/if}
					</div>

					<!-- Instructions -->
					<div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
						<h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">{t('users.howToSubscribe')}</h3>
						<div class="space-y-4 text-sm" style="color: var(--ds-text-subtle);">
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">1</span>
								<p>{t('users.copyFeedUrlStep')}</p>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">2</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">{t('users.googleCalendar')}</p>
									<p>{t('users.googleCalendarInstructions')}</p>
								</div>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">3</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">{t('users.outlook')}</p>
									<p>{t('users.outlookInstructions')}</p>
								</div>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">4</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">{t('users.appleCalendar')}</p>
									<p>{t('users.appleCalendarInstructions')}</p>
								</div>
							</div>
						</div>
					</div>

					<!-- Actions -->
					<div class="flex items-center gap-4">
						<Button
							variant="default"
							onclick={generateCalendarFeed}
							disabled={generatingFeed}
							icon={RefreshCw}
						>
							{generatingFeed ? t('common.regenerating') : t('users.regenerateUrl')}
						</Button>
						<Button
							variant="danger"
							onclick={revokeCalendarFeed}
							disabled={revokingFeed}
							icon={Trash2}
						>
							{revokingFeed ? t('common.revoking') : t('users.revokeFeed')}
						</Button>
					</div>

					<p class="text-xs" style="color: var(--ds-text-subtle);">
						<strong>{t('common.note')}:</strong> {t('users.regenerateUrlNote')}
					</p>
				</div>
			{/if}
		{/if}
	</Tabs>

</div>
