import { describe, expect, it } from 'vitest';
import { newestCommentsFirst } from './milestoneCommentOrder.js';

const comment = (id, created_at, extra = {}) => ({ id, created_at, ...extra });

describe('newestCommentsFirst', () => {
  it('sorts comments newest first', () => {
    const loaded = [
      comment(1, '2026-01-01T10:00:00Z'),
      comment(2, '2026-01-02T10:00:00Z'),
      comment(3, '2026-01-03T10:00:00Z'),
    ];
    expect(newestCommentsFirst(loaded).map((c) => c.id)).toEqual([3, 2, 1]);
  });

  it('puts a just-created comment, appended at the end, first', () => {
    const comments = [
      comment(1, '2026-01-01T10:00:00Z'),
      comment(2, '2026-01-02T10:00:00Z'),
      comment(9, '2026-02-01T10:00:00Z'),
    ];
    expect(newestCommentsFirst(comments)[0].id).toBe(9);
  });

  it('keeps an edited comment in place: updated_at does not move it', () => {
    const comments = [
      comment(1, '2026-01-01T10:00:00Z', { updated_at: '2026-03-01T10:00:00Z' }),
      comment(2, '2026-01-02T10:00:00Z'),
    ];
    expect(newestCommentsFirst(comments).map((c) => c.id)).toEqual([2, 1]);
  });

  it('breaks equal creation times by id, newest id first', () => {
    const comments = [comment(4, '2026-01-01T10:00:00Z'), comment(5, '2026-01-01T10:00:00Z')];
    expect(newestCommentsFirst(comments).map((c) => c.id)).toEqual([5, 4]);
  });

  it('does not mutate its input and tolerates a missing list', () => {
    const comments = [comment(1, '2026-01-01T10:00:00Z'), comment(2, '2026-01-02T10:00:00Z')];
    newestCommentsFirst(comments);
    expect(comments.map((c) => c.id)).toEqual([1, 2]);
    expect(newestCommentsFirst(null)).toEqual([]);
  });
});
