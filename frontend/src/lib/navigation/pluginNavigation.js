/**
 * Plugin pages in the main sidebar.
 *
 * A plugin adds an entry to the main navigation with a `main.nav` extension in its
 * manifest. The entry opens /plugins/<plugin>/<extension id>, which renders the
 * extension's `component` (an HTML file in the plugin's assets/) in an iframe, the
 * same way `admin.tab` extensions render inside the admin panel:
 *
 *   { "point": "main.nav", "id": "planner", "label": "Planner", "icon": "users",
 *     "displayMode": "iframe", "component": "index.html", "order": 10 }
 *
 * `order` sorts plugin entries (lower first, then by label). They appear after the
 * built-in entries; their visibility and data access are up to the plugin page,
 * which runs as the signed-in user like any admin tab.
 */

export const PLUGIN_NAV_POINT = 'main.nav';

/**
 * @typedef {object} PluginNavItem
 * @property {string} id           Unique sidebar id: `plugin-<plugin>-<extension>`.
 * @property {string} pluginName
 * @property {string} extensionId
 * @property {string} label
 * @property {string} [icon]       Icon name, resolved with resolvePluginIcon().
 * @property {string} [description]
 * @property {string} href         /plugins/<plugin>/<extension>
 * @property {string} src          URL of the page to load in the iframe.
 */

const SAFE_SEGMENT = /^[A-Za-z0-9._-]+$/;

/**
 * Sidebar entries from the extensions data returned by /api/plugins/extensions.
 * Entries without an id, label or plugin name, or with ids that can't be a single
 * URL segment, are skipped.
 * @param {Record<string, any[]>} extensionsData
 * @returns {PluginNavItem[]}
 */
export function pluginNavItems(extensionsData) {
  const extensions = extensionsData?.[PLUGIN_NAV_POINT] ?? [];
  return extensions
    .filter(
      (ext) =>
        ext?.label && SAFE_SEGMENT.test(ext?.id ?? '') && SAFE_SEGMENT.test(ext?.pluginName ?? '')
    )
    .map((ext) => ({
      id: `plugin-${ext.pluginName}-${ext.id}`,
      pluginName: ext.pluginName,
      extensionId: ext.id,
      label: ext.label,
      icon: ext.icon,
      description: ext.description,
      href: `/plugins/${ext.pluginName}/${ext.id}`,
      src: `/api/plugins/${ext.pluginName}/assets/${ext.component || 'index.html'}`,
      order: Number.isFinite(ext.order) ? ext.order : 0,
    }))
    .sort((a, b) => a.order - b.order || a.label.localeCompare(b.label))
    .map(({ order: _order, ...item }) => item);
}

/**
 * The sidebar entry a /plugins/<plugin>/<extension> route points at, or null when the
 * plugin is disabled, removed, or has no such `main.nav` extension.
 * @param {Record<string, any[]>} extensionsData
 * @param {string} pluginName
 * @param {string} extensionId
 * @returns {PluginNavItem | null}
 */
export function findPluginNavItem(extensionsData, pluginName, extensionId) {
  return (
    pluginNavItems(extensionsData).find(
      (item) => item.pluginName === pluginName && item.extensionId === extensionId
    ) ?? null
  );
}
