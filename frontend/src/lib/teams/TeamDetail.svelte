<script>
  import { untrack } from 'svelte';
  import IconPhoneCheck from '@tabler/icons-svelte-runes/icons/phone-check';
  import IconUsersGroup from '@tabler/icons-svelte-runes/icons/users-group';
  import IconUsers from '@tabler/icons-svelte-runes/icons/users';
  import IconStack2 from '@tabler/icons-svelte-runes/icons/stack-2';
  import IconBellRinging from '@tabler/icons-svelte-runes/icons/bell-ringing';
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import { teams } from '../api/teams.js';
  import { navigate } from '../router.js';
  import { authStore } from '../stores/auth.svelte.js';
  import { isSystemAdmin, permissionStore } from '../stores/permissions.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import BackButton from '../components/BackButton.svelte';
  import Tabs from '../components/Tabs.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import OverviewTab from './OverviewTab.svelte';
  import MembersTab from './MembersTab.svelte';
  import GroupsTab from './GroupsTab.svelte';
  import OnCallTab from './OnCallTab.svelte';
  import ServiceHoursTab from './ServiceHoursTab.svelte';

  let { teamId, section = 'overview' } = $props();

  let team = $state(null);
  let myRole = $state(null);
  let loading = $state(true);
  let error = $state('');

  const hasGlobalManage = $derived(
    $isSystemAdmin || $permissionStore.userPermissionKeys?.has('teams.manage') === true,
  );
  const canEdit = $derived(hasGlobalManage || myRole === 'admin');

  // Responses for a team the user has already left are dropped.
  async function loadTeam(id = teamId) {
    loading = true;
    try {
      const loaded = await teams.get(Number(id));
      if (id !== teamId) return;
      team = loaded;
      error = '';
    } catch (err) {
      if (id !== teamId) return;
      error = err.message || t('teams.failedToLoad');
      team = null;
    } finally {
      if (id === teamId) loading = false;
    }
  }

  async function loadMyRole(id = teamId) {
    const userId = $authStore.currentUser?.id;
    if (!userId) return;
    try {
      const myTeams = await teams.getTeamsForUser(userId);
      if (id !== teamId) return;
      const match = (myTeams || []).find((t) => t.id === Number(id));
      myRole = match?.role || null;
    } catch (err) {
      if (id === teamId) myRole = null;
    }
  }

  function changeSection(tab) {
    navigate(`/teams/${teamId}/${tab.tab}`);
  }

  const tabs = $derived([
    { id: 'overview', label: t('teams.tabs.overview'), icon: IconUsersGroup, testid: 'team-tab-overview' },
    { id: 'members', label: t('teams.tabs.members'), icon: IconUsers, badge: team?.direct_member_count, testid: 'team-tab-members' },
    { id: 'groups', label: t('teams.tabs.groups'), icon: IconStack2, badge: team?.group_count, testid: 'team-tab-groups' },
    { id: 'on-call', label: t('teams.tabs.onCall'), icon: IconBellRinging, testid: 'team-tab-on-call' },
    // Service hours are a management surface: the API only serves team admins
    // and global team managers, so the tab stays hidden for everyone else.
    ...(canEdit
      ? [{ id: 'service-hours', label: t('teams.tabs.serviceHours'), icon: IconClock, testid: 'team-tab-service-hours' }]
      : []),
  ]);

  // svelte-ignore state_referenced_locally
  let activeTab = $state(section);
  $effect(() => {
    activeTab = section;
  });

  // Switching tabs keeps this component mounted (#542), so a move to another
  // team arrives as a teamId change rather than a remount.
  $effect(() => {
    const id = teamId;
    untrack(() => {
      loadTeam(id);
      loadMyRole(id);
    });
  });

  function reload() {
    return loadTeam();
  }
</script>

<div class="space-y-4" data-testid="team-detail">
  <BackButton onclick={() => navigate('/teams')} label={t('teams.backToTeams')} />

  {#if loading}
    <div class="text-center py-8" style="color: var(--ds-text-subtle)">
      {t('teams.loading')}
    </div>
  {:else if error}
    <AlertBox message={error} />
  {:else if team}
    <PageHeader
      icon={IconPhoneCheck}
      title={team.name}
      subtitle={team.description || t('teams.noDescription')}
    />

    <Tabs {tabs} bind:activeTab onTabChange={changeSection}>
      {#if activeTab === 'members'}
        <MembersTab {team} {canEdit} onUpdated={reload} />
      {:else if activeTab === 'groups'}
        <GroupsTab {team} canEdit={hasGlobalManage || myRole === 'admin'} onUpdated={reload} />
      {:else if activeTab === 'on-call'}
        <OnCallTab {team} {canEdit} />
      {:else if activeTab === 'service-hours' && canEdit}
        <ServiceHoursTab {team} {canEdit} />
      {:else}
        <OverviewTab {team} {canEdit} onUpdated={reload} />
      {/if}
    </Tabs>
  {/if}
</div>
