<script>
	import { onMount } from 'svelte';
	import { api, getCalendarFeedToken, createCalendarFeedToken, revokeCalendarFeedToken } from '../api.js';
	import { authStore, attachmentStatus } from '../stores';
	import { User, Shield, Key, Smartphone, Trash2, Camera, Upload, Globe, CalendarDays, RefreshCw, Link2, Eye, EyeOff, Copy, GitBranch, Bot, Code, Plus, Tag, Plane, MoreHorizontal, Pencil } from '@lucide/svelte';
	import Button from '../components/Button.svelte';
	import Input from '../components/Input.svelte';
	import FileInput from '../components/FileInput.svelte';
	import Badge from '../components/Badge.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import Tabs from '../components/Tabs.svelte';
	import Spinner from '../components/Spinner.svelte';
	import AlertBox from '../components/AlertBox.svelte';
	import BasePicker from '../pickers/BasePicker.svelte';
	import FormField from '../components/FormField.svelte';
	import ConnectedAccountsTab from '../settings/ConnectedAccountsTab.svelte';
	import PersonalLabelManager from '../features/labels/PersonalLabelManager.svelte';
	import LeavePeriods from '../profile/LeavePeriods.svelte';
	import { copyToClipboard } from '../utils/clipboard.js';
	import { formatDate, formatDateSimple } from '../utils/dateFormatter.js';
	import { formatAuthenticatedInstant } from '../utils/authenticatedDateFormatter.js';
	import { t, i18n, SUPPORTED_LOCALES } from '../stores/i18n.svelte.js';
	import { confirm } from '../composables/useConfirm.js';
	import DescriptionText from '../components/DescriptionText.svelte';
	import DropdownMenu from '../layout/DropdownMenu.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import DialogFooter from '../dialogs/DialogFooter.svelte';
	import {
		isWebAuthnSupported,
		registerCredential,
		getWebAuthnErrorMessage,
		base64urlToArrayBuffer,
		arrayBufferToBase64url
	} from '../utils/webauthn-utils.js';

	let user = $state(null);
	let credentials = $state([]);
	let loading = $state(false);
	let error = $state('');
	let showAddCredential = $state(false);
	let enrollingFIDO = $state(false);
	let newCredentialName = $state('');
	let testingLogin = $state(false);
	let loginTestResult = $state('');

	// Avatar management state
	let showAvatarUpload = $state(false);
	let uploadingAvatar = $state(false);

	// Regional settings state
	let selectedTimezone = $state('UTC');
	let selectedLanguage = $state('en');
	let savingRegionalSettings = $state(false);
	let regionalSettingsSaved = $state(false);

	// Calendar feed state
	let calendarFeedInfo = $state(null);
	let loadingCalendarFeed = $state(false);
	let calendarFeedError = $state('');
	let generatingFeed = $state(false);
	let revokingFeed = $state(false);
	let showFullFeedUrl = $state(false);
	let feedUrlCopied = $state(false);

	// Tab state
	let activeTab = $state('avatar'); // Default to avatar tab

	// Use current user ID from auth store
	let currentUserId = $derived(authStore.currentUser?.id);

	// Configure tabs based on whether attachments are enabled
	let tabs = $derived([
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
			id: 'agents',
			label: t('workspaceSettings.tabs.codingAgents'),
			icon: Bot,
			testid: 'profile-tab-agents'
		},
		{
			id: 'connected-accounts',
			label: t('users.connectedAccountsTab'),
			icon: GitBranch
		},
		{
			id: 'labels',
			label: t('users.labels.tabLabel') || 'Personal labels',
			icon: Tag,
			testid: 'profile-tab-labels'
		},
		{
			id: 'calendar-integration',
			label: t('users.calendarIntegration'),
			icon: CalendarDays
		},
		{
			id: 'leave',
			label: t('profile.leave.tabLabel'),
			icon: Plane
		}
	]);

	// Agents tab state
	let agents = $state([]);
	let agentsLoading = $state(false);
	let agentCreateError = $state('');
	let newAgent = $state({ username: '', first_name: '', last_name: '', email: '' });
	let creatingAgent = $state(false);
	let featureDisabledNotice = $state(false);
	let agentTokens = $state({}); // agentId -> tokens[]
	let expandedAgent = $state(null); // which agent's tokens panel is open
	let agentMintState = $state({}); // agentId -> { name, expiresDays, minting, error, token }
	let editingAgent = $state(null);
	let editingAgentName = $state('');
	let agentRenameError = $state('');
	let renamingAgent = $state(false);

	function ensureAgentMintState(agentId) {
		if (!agentMintState[agentId]) {
			agentMintState[agentId] = { name: '', expiresAt: '', minting: false, error: '', token: '' };
		}
	}

	async function toggleAgentTokens(agentId) {
		if (expandedAgent === agentId) {
			expandedAgent = null;
			return;
		}
		expandedAgent = agentId;
		ensureAgentMintState(agentId);
		try {
			agentTokens[agentId] = await api.getApiTokens(agentId);
		} catch (err) {
			agentTokens[agentId] = [];
			console.error('Failed to load agent tokens:', err);
		}
	}

	async function mintAgentToken(agentId) {
		ensureAgentMintState(agentId);
		const s = agentMintState[agentId];
		if (!s.name.trim()) {
			s.error = t('security.enterTokenName');
			return;
		}
		s.error = '';
		s.minting = true;
		try {
			const payload = {
				name: s.name,
				user_id: agentId,
				expires_on: s.expiresAt || null
			};
			const result = await api.createApiToken(payload);
			s.token = result?.token || result?.api_token?.token || '';
			s.name = '';
			s.expiresAt = '';
			agentTokens[agentId] = await api.getApiTokens(agentId);
		} catch (err) {
			s.error = t('users.agents.tokenCreateFailed');
		} finally {
			s.minting = false;
		}
	}

	async function revokeAgentToken(agentId, tokenId, tokenName) {
		const confirmed = await confirm({
			title: t('security.revokeToken'),
			message: tokenName
				? t('security.confirmRevokeToken', { name: tokenName })
				: t('users.agents.confirmRevokeUnnamedToken'),
			confirmText: t('security.revokeToken'),
		});
		if (!confirmed) return;
		try {
			await api.revokeApiToken(tokenId);
			agentTokens[agentId] = await api.getApiTokens(agentId);
		} catch (err) {
			console.error('Failed to revoke token:', err);
		}
	}

	async function loadAgents() {
		agentsLoading = true;
		try {
			agents = await api.getMyAgents();
		} catch (err) {
			agents = [];
			console.error('Failed to load agents:', err);
		} finally {
			agentsLoading = false;
		}
	}

	async function handleCreateAgent() {
		agentCreateError = '';
		featureDisabledNotice = false;
		if (!newAgent.username || !newAgent.first_name || !newAgent.last_name) {
			agentCreateError = t('users.agents.requiredFields');
			return;
		}
		creatingAgent = true;
		try {
			const created = await api.createMyAgent({
				username: newAgent.username,
				first_name: newAgent.first_name,
				last_name: newAgent.last_name,
				email: newAgent.email || undefined
			});
			agents = [created, ...agents];
			newAgent = { username: '', first_name: '', last_name: '', email: '' };
		} catch (err) {
			// 403 from backend when flag is off or cap reached.
			if (err?.status === 403) {
				featureDisabledNotice = true;
			} else {
				agentCreateError = t('users.agents.createFailed');
			}
		} finally {
			creatingAgent = false;
		}
	}

	async function handleDeleteAgent(agentId) {
		const confirmed = await confirm({
			title: t('users.agents.deleteTitle'),
			message: t('users.agents.deleteMessage'),
			confirmText: t('common.delete'),
		});
		if (!confirmed) return;
		try {
			await api.deleteMyAgent(agentId);
			agents = agents.filter(a => a.id !== agentId);
		} catch (err) {
			console.error('Failed to delete agent:', err);
		}
	}

	function openRenameAgent(agent) {
		editingAgent = agent;
		editingAgentName = agent.full_name || `${agent.first_name} ${agent.last_name}`.trim();
		agentRenameError = '';
	}

	function closeRenameAgent() {
		if (renamingAgent) return;
		editingAgent = null;
		editingAgentName = '';
		agentRenameError = '';
	}

	async function renameAgent() {
		const name = editingAgentName.trim();
		if (!editingAgent || !name) {
			agentRenameError = t('users.agents.nameRequired');
			return;
		}
		renamingAgent = true;
		agentRenameError = '';
		try {
			const updated = await api.updateMyAgent(editingAgent.id, { name });
			agents = agents.map((agent) => agent.id === updated.id ? updated : agent);
			editingAgent = null;
			editingAgentName = '';
		} catch (err) {
			agentRenameError = t('users.agents.renameFailed');
		} finally {
			renamingAgent = false;
		}
	}

	function agentActionItems(agent) {
		return [
			{
				id: 'rename',
				title: t('users.agents.rename'),
				icon: Pencil,
				onClick: () => openRenameAgent(agent),
			},
			{
				id: 'tokens',
				testid: `agent-manage-tokens-${agent.id}`,
				title: expandedAgent === agent.id ? t('users.agents.hideTokens') : t('users.agents.manageTokens'),
				icon: Key,
				onClick: () => toggleAgentTokens(agent.id),
			},
			{ type: 'divider' },
			{
				id: 'delete',
				title: t('common.delete'),
				icon: Trash2,
				color: 'var(--ds-text-danger)',
				onClick: () => handleDeleteAgent(agent.id),
			},
		];
	}

	// Set initial active tab (avatar if attachments enabled, otherwise regional-settings)
	$effect(() => {
		if (tabs.length > 0 && !tabs.find(t => t.id === activeTab)) {
			activeTab = tabs[0].id;
		}
	});

	onMount(() => {
		if (currentUserId) {
			loadUserProfile();
			loadCredentials();
			loadAgents();
		}
	});

	// Watch for currentUserId changes and load data when available
	$effect(() => {
		if (currentUserId && !user) {
			loadUserProfile();
			loadCredentials();
			loadAgents();
		}
	});

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
				user = await api.updateUserAvatar(currentUserId, uploadResult.avatar_url);
				authStore.patchCurrentUser({ avatar_url: user.avatar_url || uploadResult.avatar_url });
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
		const avatarConfirmed = await confirm({
			title: t('common.remove'),
			message: t('dialogs.confirmations.removeAvatar'),
			confirmText: t('common.remove'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!avatarConfirmed) return;

		try {
			user = await api.updateUserAvatar(currentUserId, null);
			authStore.patchCurrentUser({ avatar_url: '' });
		} catch (err) {
			error = err.message || t('dialogs.alerts.failedToDelete', { error: 'avatar' });
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
		const credConfirmed = await confirm({
			title: t('common.delete'),
			message: t('dialogs.confirmations.deleteItem', { name: credentialName }),
			confirmText: t('common.delete'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!credConfirmed) return;

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
							type: /** @type {const} */ ('public-key'),
							id: base64urlToArrayBuffer(credData.rawId)
						};
					} catch (e) {
						console.warn('Could not parse credential data:', e);
						return null;
					}
				}).filter(Boolean),
				userVerification: /** @type {const} */ ('preferred'),
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
			const updatedUser = await api.updateUserRegionalSettings(currentUserId, {
				timezone: selectedTimezone,
				language: selectedLanguage
			});

			// Keep the global authenticated user in sync before changing locale.
			// Otherwise App.svelte observes the stale language and immediately
			// switches the interface back to it.
			const savedLanguage = updatedUser?.language || selectedLanguage;
			authStore.patchCurrentUser({
				language: savedLanguage,
				timezone: updatedUser?.timezone || selectedTimezone
			});
			await i18n.setLocale(savedLanguage);

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
		const feedConfirmed = await confirm({
			title: t('common.delete'),
			message: t('dialogs.confirmations.revokeCalendarFeed'),
			confirmText: t('common.delete'),
			cancelText: t('common.cancel'),
			variant: 'danger'
		});
		if (!feedConfirmed) return;

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

	async function copyFeedUrl() {
		if (!calendarFeedInfo?.feed?.feed_url) return;
		await copyToClipboard(calendarFeedInfo.feed.feed_url);
		feedUrlCopied = true;
		setTimeout(() => (feedUrlCopied = false), 2000);
	}

	function getMaskedFeedUrl(url) {
		if (!url) return '';
		// Show first 40 chars and last 20 chars
		if (url.length <= 70) return url;
		return url.substring(0, 40) + '...' + url.substring(url.length - 20);
	}

	// Common timezones (IANA format). Intl supplies locale-aware names while
	// the IANA identifier keeps otherwise identical regional labels distinct.
	const commonTimezoneIds = [
		'UTC',
		'America/New_York',
		'America/Chicago',
		'America/Denver',
		'America/Los_Angeles',
		'America/Anchorage',
		'Pacific/Honolulu',
		'Europe/London',
		'Europe/Paris',
		'Europe/Berlin',
		'Europe/Rome',
		'Europe/Madrid',
		'Asia/Tokyo',
		'Asia/Shanghai',
		'Asia/Hong_Kong',
		'Asia/Singapore',
		'Asia/Dubai',
		'Asia/Kolkata',
		'Australia/Sydney',
		'Australia/Melbourne',
		'Pacific/Auckland'
	];

	function getLocalizedTimezoneLabel(timezone) {
		try {
			const formatter = new Intl.DateTimeFormat(i18n.locale, {
				timeZone: timezone,
				timeZoneName: timezone === 'UTC' ? 'long' : 'longGeneric'
			});
			const name = formatter.formatToParts(new Date()).find(part => part.type === 'timeZoneName')?.value;
			if (!name) return timezone;
			return timezone === 'UTC' ? `UTC (${name})` : `${name} (${timezone})`;
		} catch {
			return timezone;
		}
	}

	let commonTimezones = $derived(
		commonTimezoneIds.map(value => ({ value, label: getLocalizedTimezoneLabel(value) }))
	);

	// Languages - only show those with UI translations
	const commonLanguages = SUPPORTED_LOCALES.map(loc => ({
		value: loc.code,
		label: loc.name
	}));
</script>

<div class="max-w-6xl mx-auto space-y-6">
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
						<Badge variant="warning" class="mt-1">
							{t('users.passwordResetRequired')}
						</Badge>
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
						<img class="h-20 w-20 rounded-full border-2" style="border-color: var(--ds-border);" src={user.avatar_url} alt={t('users.currentProfilePicture')} />
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
					<DescriptionText variant="subtlest">
						{t('users.avatarRecommendation')}
					</DescriptionText>
				</div>
			</div>

			<!-- Avatar Upload Interface -->
			{#if showAvatarUpload}
				<div class="border rounded p-4" style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border);">
					<h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('users.uploadNewAvatar')}</h3>

					<div class="mb-4">
											<FileInput
												accept="image/*"
												onchange={(e) => handleAvatarUpload(/** @type {HTMLInputElement} */ (e.target).files)}
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
			<FormField label={t('users.timezone')} id="timezone" helper={t('users.timezoneHint')}>
				<BasePicker
					bind:value={selectedTimezone}
					items={commonTimezones}
					placeholder={t('users.timezone')}
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
			</FormField>

			<!-- Language Selection -->
			<FormField label={t('users.language')} id="language" helper={t('users.languageHint')}>
				<BasePicker
					bind:value={selectedLanguage}
					items={commonLanguages}
					placeholder={t('users.language')}
					disabled={!user || savingRegionalSettings}
					getValue={(item) => item.value}
					getLabel={(item) => item.label}
				/>
			</FormField>
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

		<!-- Agents Tab -->
		{#if activeTab === 'agents'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<Bot class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					{t('users.agents.title')}
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					{t('users.agents.description')}
				</p>
			</div>

			<div class="border rounded-lg p-6 mb-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
				<h3 class="text-base font-medium mb-3" style="color: var(--ds-text);">{t('users.agents.createTitle')}</h3>
				<div class="grid grid-cols-1 md:grid-cols-2 gap-3">
					<Input placeholder={t('common.username')} bind:value={newAgent.username} />
					<Input type="email" placeholder={t('common.email')} bind:value={newAgent.email} />
					<Input placeholder={t('users.firstName')} bind:value={newAgent.first_name} />
					<Input placeholder={t('users.lastName')} bind:value={newAgent.last_name} />
				</div>
				{#if agentCreateError}
					<p class="text-sm mt-2" style="color: var(--ds-text-danger);">{agentCreateError}</p>
				{/if}
				{#if featureDisabledNotice}
					<p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
						{t('users.agents.creationUnavailable')}
					</p>
				{/if}
				<div class="mt-3">
					<Button
						variant="primary"
						onclick={handleCreateAgent}
						disabled={creatingAgent}
						loading={creatingAgent}
					>
						{creatingAgent ? t('users.agents.creating') : t('users.agents.create')}
					</Button>
				</div>
			</div>

			<div class="border rounded-lg p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
				<h3 class="text-base font-medium mb-3" style="color: var(--ds-text);">{t('users.agents.yourAgents')}</h3>
				{#if agentsLoading}
					<p class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
				{:else if agents.length === 0}
					<p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.agents.empty')}</p>
				{:else}
					<ul class="divide-y divide-[var(--ds-border)]">
						{#each agents as agent (agent.id)}
							<li data-testid={`agent-row-${agent.id}`} class="py-3">
								<div class="flex items-center justify-between">
									<div>
										<div class="font-medium" style="color: var(--ds-text);">{agent.full_name || `${agent.first_name} ${agent.last_name}`}</div>
										<div class="text-sm" style="color: var(--ds-text-subtle);">
											@{agent.username} · {agent.is_active ? t('users.active') : t('users.inactive')}
										</div>
									</div>
									<DropdownMenu
										triggerClass="p-2 rounded hover-bg transition-colors"
										showChevron={false}
										iconOnly
										items={agentActionItems(agent)}
										placement="bottom-end"
										triggerTestid={`agent-actions-${agent.id}`}
									>
										{#snippet children()}
											<MoreHorizontal class="w-5 h-5" aria-hidden="true" />
											<span class="sr-only">{t('users.agents.actionsFor', { name: agent.full_name || agent.username })}</span>
										{/snippet}
									</DropdownMenu>
								</div>

								{#if expandedAgent === agent.id && agentMintState[agent.id]}
									<div class="mt-3 p-4 rounded" style="background-color: var(--ds-background-neutral);">
										{#if agentMintState[agent.id].token}
											<div class="p-4 rounded mb-4" style="background-color: var(--ds-background-success-subtle); border: 1px solid var(--ds-border-success);">
												<h5 class="text-sm font-semibold mb-2" style="color: var(--ds-text-success);">{t('security.tokenCreated')}</h5>
												<p class="text-sm mb-3" style="color: var(--ds-text);">
													{t('security.tokenWarning')}
												</p>
												<div class="flex items-center space-x-2">
											<Input
												type="text"
												value={agentMintState[agent.id].token}
														readonly
												class="flex-1 font-mono border-[var(--ds-border-success)]"
													/>
													<Button
														variant="default"
														size="small"
														icon={Copy}
														onclick={() => copyToClipboard(agentMintState[agent.id].token)}
													>
														{t('common.copy')}
													</Button>
												</div>
												<div class="mt-3">
													<Button
														variant="default"
														size="small"
														onclick={() => (agentMintState[agent.id].token = '')}
													>
														{t('common.done')}
													</Button>
												</div>
											</div>
										{/if}

										<div class="mb-4">
											<h5 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('security.createToken')}</h5>
											<div class="grid grid-cols-1 md:grid-cols-2 gap-2 mb-2">
												<Input placeholder={t('security.tokenName')} bind:value={agentMintState[agent.id].name} />
												<Input type="date" bind:value={agentMintState[agent.id].expiresAt} />
											</div>
											<DescriptionText>{t('users.agents.tokenExpirationHint')}</DescriptionText>
											{#if agentMintState[agent.id].error}
												<p class="text-sm mt-2" style="color: var(--ds-text-danger);">{agentMintState[agent.id].error}</p>
											{/if}
											<div class="mt-3">
												<Button
													variant="primary"
													icon={Plus}
													onclick={() => mintAgentToken(agent.id)}
													disabled={agentMintState[agent.id].minting || !agentMintState[agent.id].name.trim()}
													loading={agentMintState[agent.id].minting}
												>
													{t('security.createToken')}
												</Button>
											</div>
										</div>

									<h5 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{t('users.agents.existingTokens')}</h5>
										<div class="space-y-2">
											{#each agentTokens[agent.id] || [] as tok (tok.id)}
												<div data-testid={`agent-token-row-${tok.id}`} class="flex items-center justify-between p-3 border rounded" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
													<div class="flex items-center space-x-3">
														<Code class="h-5 w-5" style="color: var(--ds-icon-subtle);" />
														<div>
															<div class="font-medium text-sm" style="color: var(--ds-text);">{tok.name}</div>
															<div class="text-xs" style="color: var(--ds-text-subtle);">
													{t('users.agents.tokenDates', {
														created: formatAuthenticatedInstant(tok.created_at, { year: 'numeric', month: 'short', day: 'numeric' }) || '-',
														expires: formatAuthenticatedInstant(tok.expires_at, { year: 'numeric', month: 'short', day: 'numeric' }) || t('users.agents.neverExpires')
													})}
															</div>
														</div>
													</div>
													<Button
														variant="default"
														size="small"
														icon={Trash2}
														dataTestid={`agent-token-revoke-${tok.id}`}
														onclick={() => revokeAgentToken(agent.id, tok.id, tok.name)}
													>
														{t('security.revokeToken')}
													</Button>
												</div>
											{:else}
										<p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.agents.noTokens')}</p>
											{/each}
										</div>
									</div>
								{/if}
							</li>
						{/each}
					</ul>
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

		<!-- Personal Labels Tab -->
		{#if activeTab === 'labels'}
			<div class="mb-6">
				<h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
					<Tag class="h-5 w-5" style="color: var(--ds-text-subtle);" />
					{t('users.labels.tabLabel')}
				</h2>
				<p class="text-sm" style="color: var(--ds-text-subtle);">
					{t('users.labels.tabDescription')}
				</p>
			</div>

			<PersonalLabelManager />
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
							<Input
								type="text"
								readonly
								value={showFullFeedUrl ? calendarFeedInfo.feed?.feed_url : getMaskedFeedUrl(calendarFeedInfo.feed?.feed_url)}
								class="flex-1 font-mono"
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
								{t('users.lastSynced')}: {formatDateSimple(calendarFeedInfo.feed.last_accessed_at)}
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

		{#if activeTab === 'leave'}
			<LeavePeriods />
		{/if}
	</Tabs>

</div>

<Modal
	isOpen={editingAgent !== null}
	preventClose={renamingAgent}
	closeOnBackdropClick={false}
	onclose={closeRenameAgent}
	onSubmit={renameAgent}
	submitDisabled={renamingAgent || !editingAgentName.trim()}
	maxWidth="max-w-md"
>
	<ModalHeader title={t('users.agents.rename')} onClose={closeRenameAgent} />
	<div class="px-6 py-4">
		<label for="agent-display-name" class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
			{t('users.displayName')}
		</label>
		<Input id="agent-display-name" bind:value={editingAgentName} maxlength={100} />
		<p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
			{t('users.agents.renameHint', { username: editingAgent?.username ?? '' })}
		</p>
		{#if agentRenameError}
			<p class="text-sm mt-2" style="color: var(--ds-text-danger);">{agentRenameError}</p>
		{/if}
	</div>
	<DialogFooter
		confirmLabel={t('users.agents.rename')}
		onCancel={closeRenameAgent}
		onConfirm={renameAgent}
		disabled={!editingAgentName.trim()}
		loading={renamingAgent}
		showKeyboardHint
		confirmTestid="agent-rename-confirm"
	/>
</Modal>
