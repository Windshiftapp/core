import { describe, expect, it } from 'vitest';
import { findPluginNavItem, PLUGIN_NAV_POINT, pluginNavItems } from './pluginNavigation.js';

const ext = (overrides = {}) => ({
  point: PLUGIN_NAV_POINT,
  id: 'planner',
  label: 'Planner',
  icon: 'users',
  displayMode: 'iframe',
  component: 'index.html',
  pluginName: 'tempo-planning',
  ...overrides,
});

describe('pluginNavItems', () => {
  it('turns main.nav extensions into sidebar entries', () => {
    expect(pluginNavItems({ [PLUGIN_NAV_POINT]: [ext()] })).toEqual([
      {
        id: 'plugin-tempo-planning-planner',
        pluginName: 'tempo-planning',
        extensionId: 'planner',
        label: 'Planner',
        icon: 'users',
        description: undefined,
        href: '/plugins/tempo-planning/planner',
        src: '/api/plugins/tempo-planning/assets/index.html',
      },
    ]);
  });

  it('ignores other extension points and missing data', () => {
    expect(pluginNavItems({ 'admin.tab': [ext()] })).toEqual([]);
    expect(pluginNavItems({})).toEqual([]);
    expect(pluginNavItems(null)).toEqual([]);
  });

  it('defaults the component to index.html', () => {
    expect(pluginNavItems({ [PLUGIN_NAV_POINT]: [ext({ component: '' })] })[0].src).toBe(
      '/api/plugins/tempo-planning/assets/index.html'
    );
  });

  it('skips entries without a label or with ids that are not a single URL segment', () => {
    const items = pluginNavItems({
      [PLUGIN_NAV_POINT]: [
        ext({ label: '' }),
        ext({ id: '../admin' }),
        ext({ id: 'a/b' }),
        ext({ pluginName: undefined }),
        ext({ id: 'ok' }),
      ],
    });
    expect(items.map((i) => i.extensionId)).toEqual(['ok']);
  });

  it('sorts by order, then label', () => {
    const items = pluginNavItems({
      [PLUGIN_NAV_POINT]: [
        ext({ id: 'c', label: 'Charlie', order: 5 }),
        ext({ id: 'b', label: 'Bravo' }),
        ext({ id: 'a', label: 'Alpha' }),
      ],
    });
    expect(items.map((i) => i.label)).toEqual(['Alpha', 'Bravo', 'Charlie']);
  });
});

describe('findPluginNavItem', () => {
  const data = { [PLUGIN_NAV_POINT]: [ext(), ext({ pluginName: 'other', id: 'planner' })] };

  it('finds the entry for a plugin route', () => {
    expect(findPluginNavItem(data, 'other', 'planner')?.id).toBe('plugin-other-planner');
  });

  it('is null for an unknown or disabled plugin', () => {
    expect(findPluginNavItem(data, 'tempo-planning', 'missing')).toBeNull();
    expect(findPluginNavItem({}, 'tempo-planning', 'planner')).toBeNull();
  });
});
