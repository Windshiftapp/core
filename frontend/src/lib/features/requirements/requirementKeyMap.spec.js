import { beforeEach, describe, expect, it, vi } from 'vitest';

const { currentRoute, isMobileRoute } = vi.hoisted(() => ({
  currentRoute: {
    subscribe: (fn) => {
      fn({ view: 'mobile-requirement-detail' });
      return () => {};
    },
  },
  isMobileRoute: (view) => String(view).startsWith('mobile-'),
}));

vi.mock('../../router.js', () => ({
  currentRoute,
  isMobileRoute,
}));

import { pageHref, traceabilityEntityHref } from './requirementKeyMap.js';

describe('traceabilityEntityHref', () => {
  it('returns mobile routes for supported entity types', () => {
    expect(traceabilityEntityHref({ type: 'item', id: 5 }, 1, null)).toBe('/m/items/5');
    expect(traceabilityEntityHref({ type: 'page', id: 9 }, 1, { requirement_number: 3 })).toBe(
      '/m/requirements/1/3'
    );
    expect(traceabilityEntityHref({ type: 'test_case', id: 11 }, 1, null)).toBe('/m/tests/1/11');
    expect(traceabilityEntityHref({ type: 'asset', id: 12 }, 1, null)).toBe('/m/assets/12');
    expect(traceabilityEntityHref({ type: 'unknown', id: 1 }, 1, null)).toBeNull();
    expect(traceabilityEntityHref({ type: 'item', id: 0 }, 1, null)).toBeNull();
  });
});

describe('pageHref', () => {
  beforeEach(() => {
    currentRoute.subscribe = (fn) => {
      fn({ view: 'mobile-requirement-detail' });
      return () => {};
    };
  });

  it('routes requirements to mobile detail on mobile views', () => {
    expect(pageHref(1, 99, { requirement_number: 4 })).toBe('/m/requirements/1/4');
    expect(pageHref(1, 99, null)).toBe('/m/pages/1/99');
  });
});
