import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  requirementMatchesQuery,
  searchRequirementsAcrossWorkspaces,
} from './mobileRequirementsData.js';

vi.mock('../api.js', () => ({
  api: {
    requirements: {
      list: vi.fn(),
    },
  },
}));

import { api } from '../api.js';

describe('requirementMatchesQuery', () => {
  it('matches key, title, and number case-insensitively', () => {
    const req = { key: 'CRM-DOC-1', page_title: 'Login flow', requirement_number: 1 };
    expect(requirementMatchesQuery(req, '')).toBe(true);
    expect(requirementMatchesQuery(req, 'crm-doc')).toBe(true);
    expect(requirementMatchesQuery(req, 'login')).toBe(true);
    expect(requirementMatchesQuery(req, '999')).toBe(false);
  });
});

describe('searchRequirementsAcrossWorkspaces', () => {
  beforeEach(() => {
    vi.mocked(api.requirements.list).mockReset();
  });

  it('returns capped cross-workspace matches', async () => {
    vi.mocked(api.requirements.list).mockImplementation(async (workspaceId) => ({
      items: [
        { key: `WS${workspaceId}-DOC-1`, page_title: `Req ${workspaceId}`, requirement_number: 1 },
      ],
      pagination: { total_items: 1 },
    }));

    const results = await searchRequirementsAcrossWorkspaces(
      [
        { id: 1, name: 'A' },
        { id: 2, name: 'B' },
      ],
      'req 2',
      { cap: 5 }
    );

    expect(results).toHaveLength(1);
    expect(results[0].workspace_id).toBe(2);
    expect(results[0].workspace_name).toBe('B');
  });

  it('returns empty for blank query', async () => {
    const results = await searchRequirementsAcrossWorkspaces([{ id: 1, name: 'A' }], '   ');
    expect(results).toEqual([]);
    expect(api.requirements.list).not.toHaveBeenCalled();
  });
});
