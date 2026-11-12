/** @vitest-environment jsdom */

import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import WorkItemRow from './WorkItemRow.svelte';

const childItem = {
  id: 42,
  workspace_id: 1,
  workspace_key: 'CHILD',
  workspace_item_number: 42,
  title: 'Implement the final step',
  parent_id: 7,
  parent_key: 'EPIC-7',
  parent_title: 'Release the new workflow',
};

afterEach(cleanup);

describe('WorkItemRow parent metadata', () => {
  it('shows the permission-checked parent key and title when requested', () => {
    render(WorkItemRow, { item: childItem, showParent: true });

    const parent = screen.getByTestId('work-item-parent-42');
    expect(screen.getByTestId('work-item-parent-container-42')).toHaveStyle({
      maxWidth: 'clamp(6rem, 20%, 16rem)',
    });
    expect(parent).toHaveTextContent('EPIC-7');
    expect(parent).toHaveTextContent('Release the new workflow');
    expect(parent).not.toHaveTextContent('CHILD-7');
  });

  it('keeps parent metadata hidden in rows that do not opt in', () => {
    render(WorkItemRow, { item: childItem });

    expect(screen.queryByTestId('work-item-parent-42')).not.toBeInTheDocument();
  });

  it('does not expose a raw parent database id when summary data is unavailable', () => {
    render(WorkItemRow, {
      item: {
        ...childItem,
        parent_key: '',
        parent_title: '',
        parent_workspace_item_number: null,
      },
      showParent: true,
    });

    expect(screen.queryByTestId('work-item-parent-42')).not.toBeInTheDocument();
  });

  it('shows additional selected backlog metadata without changing the default row', () => {
    render(WorkItemRow, {
      item: {
        ...childItem,
        priority_name: 'High',
        priority_color: '#ef4444',
        assignee_name: 'Ada Lovelace',
        due_date: '2026-11-20',
        milestones: [{ id: 3, name: 'Autumn release', category_color: '#8b5cf6' }],
      },
      showPriority: true,
      showAssignee: true,
      showDueDate: true,
      showMilestones: true,
    });

    expect(screen.getByTestId('work-item-priority-42')).toHaveTextContent('High');
    expect(screen.getByTestId('work-item-assignee-42')).toHaveTextContent('Ada Lovelace');
    expect(screen.getByTestId('work-item-due-date-42')).toBeInTheDocument();
    expect(screen.getByTestId('work-item-milestone-42')).toHaveTextContent('Autumn release');
    expect(screen.queryByTestId('work-item-parent-42')).not.toBeInTheDocument();
  });
});
