<script>
	import Bot from '@lucide/svelte/icons/bot';
	import Bell from '@lucide/svelte/icons/bell';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import Camera from '@lucide/svelte/icons/camera';
	import GitBranch from '@lucide/svelte/icons/git-branch';
	import Globe from '@lucide/svelte/icons/globe';
	import Plane from '@lucide/svelte/icons/plane';
	import User from '@lucide/svelte/icons/user';
	import { getUser } from '../api/users.js';
	import AlertBox from '../components/AlertBox.svelte';
	import Badge from '../components/Badge.svelte';
	import Tabs from '../components/Tabs.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import LeavePeriods from '../profile/LeavePeriods.svelte';
	import ProfileAgentsTab from '../profile/ProfileAgentsTab.svelte';
	import ProfileAvatarTab from '../profile/ProfileAvatarTab.svelte';
	import ProfileCalendarTab from '../profile/ProfileCalendarTab.svelte';
	import ProfileNotificationsTab from '../profile/ProfileNotificationsTab.svelte';
	import ProfileRegionalSettingsTab from '../profile/ProfileRegionalSettingsTab.svelte';
	import ConnectedAccountsTab from '../settings/ConnectedAccountsTab.svelte';
	import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';
	import { authStore } from '../stores/auth.svelte.js';
	import { t } from '../stores/i18n.svelte.js';

	let user = $state(null);
	let error = $state('');
	let activeTab = $state('avatar');
	let loadedUserId = null;
	let loadVersion = 0;

	let currentUserId = $derived(authStore.currentUser?.id);
	let tabs = $derived([
		...(attachmentStatus.enabled
			? [{ id: 'avatar', label: t('users.avatar'), icon: Camera }]
			: []),
		{ id: 'regional-settings', label: t('users.regionalSettings'), icon: Globe },
		{
			id: 'notifications',
			label: t('users.notificationsTab'),
			icon: Bell,
			testid: 'profile-tab-notifications'
		},
		{
			id: 'agents',
			label: t('workspaceSettings.tabs.codingAgents'),
			icon: Bot,
			testid: 'profile-tab-agents'
		},
		{ id: 'connected-accounts', label: t('users.connectedAccountsTab'), icon: GitBranch },
		{ id: 'calendar-integration', label: t('users.calendarIntegration'), icon: CalendarDays },
		{ id: 'leave', label: t('profile.leave.tabLabel'), icon: Plane }
	]);

	$effect(() => {
		if (tabs.length > 0 && !tabs.some((tab) => tab.id === activeTab)) {
			activeTab = tabs[0].id;
		}
	});

	$effect(() => {
		const userId = currentUserId;
		if (!userId) {
			user = null;
			loadedUserId = null;
			return;
		}
		if (userId === loadedUserId) return;
		loadedUserId = userId;
		void loadUserProfile(userId);
	});

	async function loadUserProfile(userId) {
		const version = ++loadVersion;
		error = '';
		try {
			const loadedUser = await getUser(userId);
			if (version === loadVersion && userId === currentUserId) {
				user = loadedUser;
			}
		} catch {
			if (version === loadVersion && userId === currentUserId) {
				error = t('dialogs.alerts.failedToLoad', { error: 'user profile' });
			}
		}
	}
</script>

<div class="max-w-6xl mx-auto space-y-6">
	<PageHeader icon={User} title={t('users.profile')} subtitle={t('users.profileSubtitle')} />

	<div
		class="shadow rounded p-6 border"
		style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
	>
		<h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">
			{t('users.profileInformation')}
		</h2>
		{#if user}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">
						{t('users.fullName')}
					</span>
					<p class="mt-1 text-sm" style="color: var(--ds-text);">{user.full_name}</p>
				</div>
				<div>
					<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">
						{t('common.email')}
					</span>
					<p class="mt-1 text-sm" style="color: var(--ds-text);">{user.email}</p>
				</div>
				{#if user.requires_password_reset}
					<div>
						<span class="block text-sm font-medium" style="color: var(--ds-text-subtle);">
							{t('common.status')}
						</span>
						<Badge variant="warning" class="mt-1">{t('users.passwordResetRequired')}</Badge>
					</div>
				{/if}
			</div>
		{:else}
			<div class="animate-pulse space-y-4">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<div
							class="h-4 rounded w-16 mb-2"
							style="background-color: var(--ds-background-neutral);"
						></div>
						<div
							class="h-4 rounded w-32"
							style="background-color: var(--ds-background-neutral);"
						></div>
					</div>
					<div>
						<div
							class="h-4 rounded w-12 mb-2"
							style="background-color: var(--ds-background-neutral);"
						></div>
						<div
							class="h-4 rounded w-48"
							style="background-color: var(--ds-background-neutral);"
						></div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#if error}
		<AlertBox message={error} />
	{/if}

	<Tabs {tabs} bind:activeTab>
		<div class:hidden={activeTab !== 'avatar' || !attachmentStatus.enabled}>
			<ProfileAvatarTab userId={currentUserId} bind:user />
		</div>
		<div class:hidden={activeTab !== 'regional-settings'}>
			<ProfileRegionalSettingsTab userId={currentUserId} bind:user />
		</div>
		<div class:hidden={activeTab !== 'agents'}>
			<ProfileAgentsTab userId={currentUserId} />
		</div>
		<div class:hidden={activeTab !== 'calendar-integration'}>
			<ProfileCalendarTab />
		</div>
		<div class:hidden={activeTab !== 'notifications'}>
			<ProfileNotificationsTab />
		</div>

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
		{:else if activeTab === 'leave'}
			<LeavePeriods />
		{/if}
	</Tabs>
</div>
