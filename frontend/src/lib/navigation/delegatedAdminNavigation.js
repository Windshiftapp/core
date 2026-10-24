import IconLifebuoy from '@tabler/icons-svelte-runes/icons/lifebuoy';
import IconPackage from '@tabler/icons-svelte-runes/icons/package';
import { derived } from 'svelte/store';
import { permissionStore } from '../stores/permissions.svelte.js';

/**
 * Management surfaces collected for non-system-admins. Each item is gated by a
 * truthy key on the permission store. That store is hydrated from the
 * once-per-login permission profile and shell-bootstrap snapshots, so filtering
 * this list never issues its own request. Do not add a capability here that
 * would need a per-render query; extend the shell bootstrap instead.
 *
 * @typedef {Object} DelegatedAdminItem
 * @property {string} id
 * @property {any} icon
 * @property {string} labelKey   i18n key resolved at render time via t().
 * @property {string} href
 * @property {string} permission Truthy key on permissionStore.
 */

/** @type {DelegatedAdminItem[]} */
export const delegatedAdminItems = [
  {
    id: 'channels',
    icon: IconLifebuoy,
    labelKey: 'nav.channels',
    href: '/manage/channels',
    permission: 'canManageChannels',
  },
  {
    id: 'assets',
    icon: IconPackage,
    labelKey: 'nav.assets',
    href: '/manage/assets',
    permission: 'canManageAssets',
  },
];

/**
 * Resolve the visible delegated-admin items from a permission-store snapshot.
 * System admins use the administration panel, so they never get the delegated
 * surface even when their capability flags are set.
 *
 * @param {any} permissionState
 * @returns {DelegatedAdminItem[]}
 */
export function filterDelegatedAdminItems(permissionState) {
  if (!permissionState || permissionState.isSystemAdmin) return [];
  return delegatedAdminItems.filter((item) => permissionState[item.permission] === true);
}

export const visibleDelegatedAdminItems = derived(permissionStore, filterDelegatedAdminItems);
export const hasDelegatedAdmin = derived(visibleDelegatedAdminItems, ($items) => $items.length > 0);
