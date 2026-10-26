import { describe, expect, it } from 'vitest';
import {
  milestoneLastUpdated,
  milestoneStatusSortValue,
  milestoneTimelineSortValue,
  timestampSortValue,
} from './milestoneListSort.js';

describe('milestone list sorting', () => {
  it('takes the later of the last activity and the milestone updated_at', () => {
    expect(
      milestoneLastUpdated({
        updated_at: '2026-01-01T00:00:00Z',
        last_activity_at: '2026-02-01T00:00:00Z',
      })
    ).toBe('2026-02-01T00:00:00Z');
    expect(
      milestoneLastUpdated({
        updated_at: '2026-03-01T00:00:00Z',
        last_activity_at: '2026-02-01T00:00:00Z',
      })
    ).toBe('2026-03-01T00:00:00Z');
    // No activity yet: the milestone's own updated_at.
    expect(milestoneLastUpdated({ updated_at: '2026-03-01T00:00:00Z' })).toBe(
      '2026-03-01T00:00:00Z'
    );
    expect(milestoneLastUpdated({})).toBeNull();
  });

  it('turns dates into comparable numbers', () => {
    expect(timestampSortValue('2026-01-02T00:00:00Z')).toBeGreaterThan(
      timestampSortValue('2026-01-01T23:59:59Z')
    );
    expect(timestampSortValue(null)).toBeNull();
    expect(timestampSortValue('not a date')).toBeNull();
  });

  it('sorts statuses in lifecycle order', () => {
    const order = ['cancelled', 'planning', 'completed', 'in-progress']
      .map((status) => ({ status }))
      .sort((a, b) => milestoneStatusSortValue(a) - milestoneStatusSortValue(b))
      .map((milestone) => milestone.status);
    expect(order).toEqual(['planning', 'in-progress', 'completed', 'cancelled']);
    expect(milestoneStatusSortValue({ status: 'unknown' })).toBeNull();
  });

  it('sorts the timeline by target date of open milestones only', () => {
    expect(milestoneTimelineSortValue({ status: 'planning', target_date: '2026-05-01' })).toBe(
      Date.parse('2026-05-01')
    );
    expect(
      milestoneTimelineSortValue({ status: 'completed', target_date: '2026-05-01' })
    ).toBeNull();
    expect(
      milestoneTimelineSortValue({ status: 'cancelled', target_date: '2026-05-01' })
    ).toBeNull();
    expect(milestoneTimelineSortValue({ status: 'in-progress' })).toBeNull();
  });
});
