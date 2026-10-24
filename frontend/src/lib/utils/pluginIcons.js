import IconBell from '@tabler/icons-svelte-runes/icons/bell';
import IconClipboardList from '@tabler/icons-svelte-runes/icons/clipboard-list';
import IconCloudDownload from '@tabler/icons-svelte-runes/icons/cloud-download';
import IconDatabase from '@tabler/icons-svelte-runes/icons/database';
import IconEye from '@tabler/icons-svelte-runes/icons/eye';
import IconFileSearch from '@tabler/icons-svelte-runes/icons/file-search';
import IconFileText from '@tabler/icons-svelte-runes/icons/file-text';
import IconFolderSearch from '@tabler/icons-svelte-runes/icons/folder-search';
import IconGitBranch from '@tabler/icons-svelte-runes/icons/git-branch';
import IconKey from '@tabler/icons-svelte-runes/icons/key';
import IconLink from '@tabler/icons-svelte-runes/icons/link';
import IconLock from '@tabler/icons-svelte-runes/icons/lock';
import IconPackage from '@tabler/icons-svelte-runes/icons/package';
import IconPuzzle from '@tabler/icons-svelte-runes/icons/puzzle';
import IconSettings from '@tabler/icons-svelte-runes/icons/settings';
import IconShield from '@tabler/icons-svelte-runes/icons/shield';
import IconShieldCheck from '@tabler/icons-svelte-runes/icons/shield-check';
import IconShieldLock from '@tabler/icons-svelte-runes/icons/shield-lock';
import IconUsers from '@tabler/icons-svelte-runes/icons/users';
import IconUsersGroup from '@tabler/icons-svelte-runes/icons/users-group';

const pluginIconMap = {
  'shield-check': IconShieldCheck,
  'folder-search': IconFolderSearch,
  'users-group': IconUsersGroup,
  'file-search': IconFileSearch,
  key: IconKey,
  'shield-lock': IconShieldLock,
  puzzle: IconPuzzle,
  database: IconDatabase,
  settings: IconSettings,
  bell: IconBell,
  users: IconUsers,
  lock: IconLock,
  'file-text': IconFileText,
  'git-branch': IconGitBranch,
  'cloud-download': IconCloudDownload,
  link: IconLink,
  package: IconPackage,
  shield: IconShield,
  eye: IconEye,
  'clipboard-list': IconClipboardList,
};

/**
 * Resolve a kebab-case icon name to a Tabler icon component.
 * @param {string} iconName - kebab-case icon name (e.g. "shield-check")
 * @param {any} fallback - fallback component if icon is not found
 * @returns {any} Tabler icon component
 */
export function resolvePluginIcon(iconName, fallback) {
  if (!iconName) return fallback;
  return pluginIconMap[iconName] || fallback;
}
