<script>
	import { onMount } from 'svelte';
	import { api, getCalendarFeedToken, createCalendarFeedToken, revokeCalendarFeedToken } from '../api.js';
	import { authStore } from '../stores';
	import { User, Shield, Key, Smartphone, Plus, Trash2, Calendar, CheckCircle, PlayCircle, Code, Copy, Eye, EyeOff, Camera, Upload, Globe, CalendarDays, RefreshCw, Link2, ExternalLink, GitBranch } from 'lucide-svelte';
	import Button from '../components/Button.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import Tabs from '../components/Tabs.svelte';
	import Spinner from '../components/Spinner.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import BasePicker from '../pickers/BasePicker.svelte';
	import ConnectedAccountsTab from '../settings/ConnectedAccountsTab.svelte';
	import { formatDate } from '../utils/dateFormatter.js';
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
	let attachmentSettings = null;
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

	// Check if attachments are enabled for avatars
	$: attachmentsEnabled = attachmentSettings?.enabled && attachmentSettings?.attachment_path;

	// Configure tabs based on whether attachments are enabled
	$: tabs = [
		...(attachmentsEnabled ? [{
			id: 'avatar',
			label: 'Avatar',
			icon: Camera
		}] : []),
		{
			id: 'regional-settings',
			label: 'Regional Settings',
			icon: Globe
		},
		{
			id: 'connected-accounts',
			label: 'Connected Accounts',
			icon: GitBranch
		},
		{
			id: 'calendar-integration',
			label: 'Calendar Integration',
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
		loadAttachmentSettings();
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
			error = 'Failed to load user profile';
		}
	}

	async function loadCredentials() {
		if (!currentUserId) return;
		try {
			credentials = await api.getUserCredentials(currentUserId);
		} catch (err) {
			error = 'Failed to load credentials';
		}
	}

	async function loadAttachmentSettings() {
		try {
			attachmentSettings = await api.attachmentSettings.get();
		} catch (err) {
			console.warn('Attachments not available:', err);
		}
	}

	async function handleAvatarUpload(files) {
		if (!currentUserId || !files || files.length === 0) return;

		const file = files[0];
		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file';
			return;
		}

		uploadingAvatar = true;
		try {
			const formData = new FormData();
			formData.append('file', file);
			formData.append('item_id', '0'); // Use 0 for avatar uploads
			formData.append('category', 'avatar');

			const response = await fetch('/api/attachments/upload', {
				method: 'POST',
				body: formData,
			});

			if (!response.ok) {
				throw new Error(`Upload failed: ${response.statusText}`);
			}

			const uploadResult = await response.json();
			
			if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
				// Use the avatar_url directly from the upload response
				await api.updateUserAvatar(currentUserId, uploadResult.avatar_url);
				
				// Reload user profile to show new avatar
				await loadUserProfile();
				showAvatarUpload = false;
			}
		} catch (err) {
			error = err.message || 'Failed to upload avatar';
		} finally {
			uploadingAvatar = false;
		}
	}

	async function removeAvatar() {
		if (!currentUserId) return;
		if (!confirm('Are you sure you want to remove your profile picture?')) return;

		try {
			await api.updateUserAvatar(currentUserId, null);
			await loadUserProfile();
		} catch (err) {
			error = err.message || 'Failed to remove avatar';
		}
	}

	async function loadAppTokens() {
		if (!currentUserId) return;
		try {
			appTokens = await api.getUserAppTokens(currentUserId);
		} catch (err) {
			error = 'Failed to load app tokens';
		}
	}

	async function enrollFIDOKey() {
		if (!newCredentialName.trim()) {
			error = 'Please enter a name for this security key';
			return;
		}

		// Check if WebAuthn is supported
		if (!isWebAuthnSupported()) {
			error = 'WebAuthn is not supported in this browser';
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
				throw new Error('Invalid registration challenge from server');
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
		if (!confirm(`Are you sure you want to remove "${credentialName}"?`)) return;
		
		try {
			await api.removeUserCredential(currentUserId, credentialId);
			await loadCredentials();
		} catch (err) {
			error = err.message || 'Failed to remove credential';
		}
	}

	async function testFIDOLogin() {
		testingLogin = true;
		loginTestResult = '';
		error = '';

		try {
			// Check if WebAuthn is supported
			if (!window.PublicKeyCredential) {
				throw new Error('WebAuthn is not supported in this browser');
			}

			// Check if user has any FIDO credentials
			const fidoCredentials = credentials.filter(c => c.credential_type === 'fido' && c.is_active);
			if (fidoCredentials.length === 0) {
				throw new Error('No active FIDO credentials found. Please register a security key first.');
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
				throw new Error('Authentication failed');
			}

		} catch (err) {
			loginTestResult = 'error';
			if (err.name === 'NotAllowedError') {
				error = 'Authentication was cancelled or failed. Please try again.';
			} else {
				error = err.message || 'Failed to test FIDO login';
			}
		} finally {
			testingLogin = false;
		}
	}

	async function createAppToken() {
		if (!newTokenName.trim()) {
			error = 'Please enter a name for this token';
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
			error = err.message || 'Failed to create app token';
		} finally {
			creatingToken = false;
		}
	}

	async function revokeAppToken(tokenId, tokenName) {
		if (!confirm(`Are you sure you want to revoke "${tokenName}"? This action cannot be undone.`)) return;
		
		try {
			await api.revokeAppToken(currentUserId, tokenId);
			await loadAppTokens();
		} catch (err) {
			error = err.message || 'Failed to revoke token';
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
				return 'Security Key (FIDO2)';
			case 'totp':
				return 'Authenticator App (TOTP)';
			default:
				return 'Unknown';
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
			await loadUserProfile();

			regionalSettingsSaved = true;
			setTimeout(() => {
				regionalSettingsSaved = false;
			}, 3000);
		} catch (err) {
			error = err.message || 'Failed to save regional settings';
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
				calendarFeedError = 'Calendar feeds are disabled by your administrator.';
			} else {
				calendarFeedError = err.message || 'Failed to load calendar feed info';
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
				calendarFeedError = 'Calendar feeds are disabled by your administrator.';
			} else {
				calendarFeedError = err.message || 'Failed to generate calendar feed';
			}
		} finally {
			generatingFeed = false;
		}
	}

	async function revokeCalendarFeed() {
		if (!confirm('Are you sure you want to revoke your calendar feed URL? Any calendars using this URL will stop syncing.')) {
			return;
		}

		revokingFeed = true;
		calendarFeedError = '';
		try {
			await revokeCalendarFeedToken();
			calendarFeedInfo = { has_token: false };
			showFullFeedUrl = false;
		} catch (err) {
			calendarFeedError = err.message || 'Failed to revoke calendar feed';
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

	// Common languages (ISO 639-1 codes)
	const commonLanguages = [
		{ value: 'en', label: 'English' },
		{ value: 'de', label: 'Deutsch (German)' },
		{ value: 'es', label: 'Español (Spanish)' },
		{ value: 'fr', label: 'Français (French)' },
		{ value: 'it', label: 'Italiano (Italian)' },
		{ value: 'pt', label: 'Português (Portuguese)' },
		{ value: 'nl', label: 'Nederlands (Dutch)' },
		{ value: 'pl', label: 'Polski (Polish)' },
		{ value: 'ru', label: 'Русский (Russian)' },
		{ value: 'ja', label: '日本語 (Japanese)' },
		{ value: 'zh', label: '中文 (Chinese)' },
		{ value: 'ko', label: '한국어 (Korean)' },
		{ value: 'ar', label: 'العربية (Arabic)' },
		{ value: 'hi', label: 'हिन्दी (Hindi)' }
	];
</script>

<div class="max-w-4xl mx-auto px-6 py-8 space-y-6">
	<!-- Page Header -->
	<PageHeader
		icon={User}
		title="Profile"
		subtitle="Manage your profile information, avatar, and regional settings"
	/>

	<!-- Profile Information -->
	<div class="bg-white shadow rounded p-6" style="background-color: var(--ds-surface-raised);">
		<h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Profile Information</h2>
		{#if user}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<span class="block text-sm font-medium text-gray-700">Full Name</span>
					<p class="mt-1 text-sm text-gray-900">{user.full_name}</p>
				</div>
				<div>
					<span class="block text-sm font-medium text-gray-700">Email</span>
					<p class="mt-1 text-sm text-gray-900">{user.email}</p>
				</div>
				{#if user.requires_password_reset}
					<div>
						<span class="block text-sm font-medium text-gray-700">Status</span>
						<span class="mt-1 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800">
							Password Reset Required
						</span>
					</div>
				{/if}
			</div>
		{:else}
			<div class="animate-pulse space-y-4">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<div class="h-4 bg-gray-300 rounded w-16 mb-2"></div>
						<div class="h-4 bg-gray-300 rounded w-32"></div>
					</div>
					<div>
						<div class="h-4 bg-gray-300 rounded w-12 mb-2"></div>
						<div class="h-4 bg-gray-300 rounded w-48"></div>
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
		{#if activeTab === 'avatar' && attachmentsEnabled}
			<div class="flex items-center justify-between mb-6">
				<div>
					<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
						<Camera class="h-5 w-5" style="color: var(--ds-text-subtle);" />
						Profile Picture
					</h2>
					<p class="text-sm" style="color: var(--ds-text-subtle);">Upload and manage your avatar image</p>
				</div>
				<div class="flex items-center gap-2">
					{#if user?.avatar_url}
						<Button
							variant="default"
							onclick={removeAvatar}
							icon={Trash2}
							size="medium"
						>
							Remove
						</Button>
					{/if}
					<Button
						variant="primary"
						onclick={() => showAvatarUpload = !showAvatarUpload}
						icon={Upload}
						size="medium"
					>
						{user?.avatar_url ? 'Change Avatar' : 'Upload Avatar'}
					</Button>
				</div>
			</div>

			<!-- Current Avatar Display -->
			<div class="flex items-center gap-6 mb-6">
				<div class="relative">
					{#if user?.avatar_url}
						<img class="h-20 w-20 rounded-full border-2 border-gray-200" src={user.avatar_url} alt="Current avatar" />
					{:else}
						<div class="h-20 w-20 rounded-full bg-gray-200 flex items-center justify-center border-2 border-gray-300">
							<User class="h-10 w-10 text-gray-500" />
						</div>
					{/if}
				</div>
				<div>
					<h3 class="font-medium text-gray-900">Current Profile Picture</h3>
					<p class="text-sm text-gray-600 mt-1">
						{user?.avatar_url ? 'Your custom avatar is active' : 'Using default avatar'}
					</p>
					<p class="text-xs text-gray-500 mt-1">
						Recommended: Square image, at least 200x200 pixels
					</p>
				</div>
			</div>

			<!-- Avatar Upload Interface -->
			{#if showAvatarUpload}
				<div class="bg-gray-50 border border-gray-200 rounded p-4">
					<h3 class="text-sm font-medium text-gray-900 mb-3">Upload New Avatar</h3>
					
					<div class="mb-4">
						<input
							type="file"
							accept="image/*"
							onchange={(e) => handleAvatarUpload(e.target.files)}
							disabled={uploadingAvatar}
							class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
						/>
						<p class="text-xs text-gray-500 mt-2">
							Select an image file (JPEG, PNG, GIF, WebP). Maximum 50MB.
						</p>
					</div>

					{#if uploadingAvatar}
						<div class="mb-4">
							<div class="flex items-center gap-2 text-sm text-gray-600">
								<Spinner size="sm" />
								Uploading avatar...
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
							Cancel
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
					Regional Settings
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">Configure your timezone and language preferences</p>
			</div>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
			<!-- Timezone Selection -->
			<div>
				<label for="timezone" class="block text-sm font-medium text-gray-700 mb-2">
					Timezone
				</label>
				<BasePicker
					bind:value={selectedTimezone}
					items={commonTimezones}
					placeholder="Select timezone"
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
				<p class="text-xs text-gray-500 mt-2">
					Used for displaying dates and times in your local timezone
				</p>
			</div>

			<!-- Language Selection -->
			<div>
				<label for="language" class="block text-sm font-medium text-gray-700 mb-2">
					Language
				</label>
				<BasePicker
					bind:value={selectedLanguage}
					items={commonLanguages}
					placeholder="Select language"
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
				<p class="text-xs text-gray-500 mt-2">
					Your preferred language for the application interface
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
				{savingRegionalSettings ? 'Saving...' : 'Save Settings'}
			</Button>

			{#if regionalSettingsSaved}
				<AlertBox variant="success" message="Settings saved successfully" />
			{/if}
		</div>
		{/if}

		<!-- Connected Accounts Tab -->
		{#if activeTab === 'connected-accounts'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<GitBranch class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					Connected Accounts
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					Connect your source control accounts to create branches and pull requests
				</p>
			</div>

			<ConnectedAccountsTab />
		{/if}

		<!-- Calendar Integration Tab -->
		{#if activeTab === 'calendar-integration'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<CalendarDays class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					Calendar Integration
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					Subscribe to your scheduled items in external calendar apps like Google Calendar, Outlook, or Apple Calendar
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
						Load Calendar Feed Settings
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
							<h3 class="text-base font-medium" style="color: var(--ds-text);">Enable Calendar Subscription</h3>
							<p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
								Generate a subscription URL to sync your scheduled work items with external calendar applications.
								This creates a one-way feed that updates automatically.
							</p>
							<div class="mt-4">
								<Button
									variant="primary"
									onclick={generateCalendarFeed}
									disabled={generatingFeed}
									icon={CalendarDays}
								>
									{generatingFeed ? 'Generating...' : 'Generate Calendar Feed URL'}
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
							<h3 class="text-base font-medium" style="color: var(--ds-text);">Your Calendar Feed URL</h3>
							<div class="flex items-center gap-2">
								<button
									class="text-sm px-2 py-1 rounded hover-bg"
									style="color: var(--ds-link);"
									onclick={() => showFullFeedUrl = !showFullFeedUrl}
								>
									{#if showFullFeedUrl}
										<EyeOff class="w-4 h-4 inline mr-1" />
										Hide
									{:else}
										<Eye class="w-4 h-4 inline mr-1" />
										Show Full URL
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
								{feedUrlCopied ? 'Copied!' : 'Copy'}
							</Button>
						</div>

						<p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
							Add this URL to your calendar app as a subscription/ICS feed. Do not share this URL as it provides access to your scheduled items.
						</p>

						{#if calendarFeedInfo.feed?.last_accessed_at}
							<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
								Last synced: {formatDate(calendarFeedInfo.feed.last_accessed_at, { relative: true })}
							</p>
						{/if}
					</div>

					<!-- Instructions -->
					<div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
						<h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">How to Subscribe</h3>
						<div class="space-y-4 text-sm" style="color: var(--ds-text-subtle);">
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">1</span>
								<p>Copy the feed URL above</p>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">2</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">Google Calendar</p>
									<p>Settings &gt; Add calendar &gt; From URL &gt; Paste the URL</p>
								</div>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">3</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">Outlook</p>
									<p>Add calendar &gt; Subscribe from web &gt; Paste the URL</p>
								</div>
							</div>
							<div class="flex items-start gap-3">
								<span class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">4</span>
								<div>
									<p class="font-medium" style="color: var(--ds-text);">Apple Calendar</p>
									<p>File &gt; New Calendar Subscription &gt; Paste the URL</p>
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
							{generatingFeed ? 'Regenerating...' : 'Regenerate URL'}
						</Button>
						<Button
							variant="danger"
							onclick={revokeCalendarFeed}
							disabled={revokingFeed}
							icon={Trash2}
						>
							{revokingFeed ? 'Revoking...' : 'Revoke Feed'}
						</Button>
					</div>

					<p class="text-xs" style="color: var(--ds-text-subtle);">
						<strong>Note:</strong> Regenerating the URL will invalidate your current URL. Any calendars using the old URL will need to be updated.
					</p>
				</div>
			{/if}
		{/if}
	</Tabs>

</div>
