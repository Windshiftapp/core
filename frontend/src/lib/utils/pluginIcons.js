import {
  IconShieldCheck, IconFolderSearch, IconUsersGroup, IconFileSearch,
  IconKey, IconShieldLock, IconPuzzle, IconDatabase, IconSettings,
  IconBell, IconUsers, IconLock, IconFileText, IconGitBranch,
  IconCloudDownload, IconLink, IconPackage, IconShield, IconEye,
  IconClipboardList
} from '@tabler/icons-svelte-runes';

const pluginIconMap = {
  'shield-check': IconShieldCheck,
  'folder-search': IconFolderSearch,
  'users-group': IconUsersGroup,
  'file-search': IconFileSearch,
  'key': IconKey,
  'shield-lock': IconShieldLock,
  'puzzle': IconPuzzle,
  'database': IconDatabase,
  'settings': IconSettings,
  'bell': IconBell,
  'users': IconUsers,
  'lock': IconLock,
  'file-text': IconFileText,
  'git-branch': IconGitBranch,
  'cloud-download': IconCloudDownload,
  'link': IconLink,
  'package': IconPackage,
  'shield': IconShield,
  'eye': IconEye,
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
