<script>
	import { Bot, CalendarDays, Camera, GitBranch, Globe, Plane, Tag, User } from '@lucide/svelte';
	import { api } from '../api.js';
	import AlertBox from '../components/AlertBox.svelte';
	import Badge from '../components/Badge.svelte';
	import Tabs from '../components/Tabs.svelte';
	import PersonalLabelManager from '../features/labels/PersonalLabelManager.svelte';
	import PageHeader from '../layout/PageHeader.svelte';
	import LeavePeriods from '../profile/LeavePeriods.svelte';
	import ProfileAgentsTab from '../profile/ProfileAgentsTab.svelte';
	import ProfileAvatarTab from '../profile/ProfileAvatarTab.svelte';
	import ProfileCalendarTab from '../profile/ProfileCalendarTab.svelte';
	import ProfileRegionalSettingsTab from '../profile/ProfileRegionalSettingsTab.svelte';
	import ConnectedAccountsTab from '../settings/ConnectedAccountsTab.svelte';
	import { attachmentStatus, authStore } from '../stores';
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
			id: 'agents',
			label: t('workspaceSettings.tabs.codingAgents'),
			icon: Bot,
			testid: 'profile-tab-agents'
		},
		{ id: 'connected-accounts', label: t('users.connectedAccountsTab'), icon: GitBranch },
		{
			id: 'labels',
			label: t('users.labels.tabLabel') || 'Personal labels',
			icon: Tag,
			testid: 'profile-tab-labels'
		},
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
			const loadedUser = await api.getUser(userId);
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
		{:else if activeTab === 'labels'}
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
		{:else if activeTab === 'leave'}
			<LeavePeriods />
		{/if}
	</Tabs>
</div>
