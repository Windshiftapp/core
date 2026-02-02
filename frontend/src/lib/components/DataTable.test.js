import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import DataTable from './DataTable.svelte';

// Mock the lucide-svelte icons
vi.mock('lucide-svelte', () => ({
  MoreHorizontal: vi.fn(() => null),
  ChevronLeft: vi.fn(() => null),
  ChevronRight: vi.fn(() => null),
}));

// Mock child components - for Svelte 5, provide minimal component mocks
vi.mock('./DropdownMenu.svelte', () => {
  return {
    default: function MockDropdownMenu() { },
  };
});

vi.mock('./EmptyState.svelte', () => {
  return {
    default: function MockEmptyState() { },
  };
});

// Mock the i18n store
vi.mock('../stores/i18n.svelte.js', () => ({
  t: vi.fn((key, params = {}) => {
    const translations = {
      'components.dataTable.showingRange': `Showing ${params.start}–${params.end} of ${params.total}`,
      'components.pagination.pageOf': `Page ${params.current} of ${params.total}`,
      'common.noData': 'No data available'
    };
    return translations[key] || key;
  })
}));

describe('DataTable', () => {
  const sampleColumns = [
    { key: 'name', label: 'Name' },
    { key: 'email', label: 'Email' },
    { key: 'status', label: 'Status' },
  ];

  const sampleData = [
    { id: 1, name: 'Alice', email: 'alice@example.com', status: 'Active' },
    { id: 2, name: 'Bob', email: 'bob@example.com', status: 'Inactive' },
    { id: 3, name: 'Charlie', email: 'charlie@example.com', status: 'Active' },
  ];

  describe('Rendering', () => {
    it('should render correct number of rows', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      // Find all tbody rows
      const rows = document.querySelectorAll('tbody tr');
      expect(rows.length).toBe(3);
    });

    it('should render correct number of columns', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      // Find all header cells
      const headerCells = document.querySelectorAll('thead th');
      expect(headerCells.length).toBe(3);
    });

    it('should render column headers correctly', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      expect(screen.getByText('Name')).toBeInTheDocument();
      expect(screen.getByText('Email')).toBeInTheDocument();
      expect(screen.getByText('Status')).toBeInTheDocument();
    });

    it('should render cell values correctly', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      expect(screen.getByText('Alice')).toBeInTheDocument();
      expect(screen.getByText('alice@example.com')).toBeInTheDocument();
      expect(screen.getByText('Bob')).toBeInTheDocument();
    });
  });

  describe('Empty State', () => {
    it('should render empty state when data is empty', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: [],
          emptyMessage: 'No users found',
        },
      });

      // Table should not be rendered
      const table = document.querySelector('table');
      expect(table).toBeNull();
    });

    it('should not render empty state when data exists', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      const table = document.querySelector('table');
      expect(table).toBeInTheDocument();
    });
  });

  describe('XSS Protection', () => {
    it('should escape HTML content by default (text rendering)', async () => {
      const xssData = [
        { id: 1, name: '<script>alert("xss")</script>', email: 'test@test.com', status: 'Active' },
      ];

      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: xssData,
        },
      });

      // The script tag should be rendered as text, not executed
      const cell = screen.getByText('<script>alert("xss")</script>');
      expect(cell).toBeInTheDocument();

      // Verify it's rendered as text content, not as HTML
      expect(cell.innerHTML).not.toContain('<script>');
    });

    it('should escape HTML in render function output by default', async () => {
      const xssColumns = [
        {
          key: 'name',
          label: 'Name',
          render: (item) => `<img src=x onerror="alert('xss')">`,
        },
      ];

      render(DataTable, {
        props: {
          columns: xssColumns,
          data: [{ id: 1, name: 'Test' }],
        },
      });

      // The img tag should be rendered as text
      const cells = document.querySelectorAll('td');
      expect(cells[0].textContent).toContain('<img src=x onerror=');
      expect(cells[0].innerHTML).not.toContain('<img');
    });

    it('should only render HTML when column.html is explicitly true', async () => {
      const htmlColumns = [
        {
          key: 'formatted',
          label: 'Formatted',
          html: true,
          render: (item) => `<strong>${item.name}</strong>`,
        },
      ];

      render(DataTable, {
        props: {
          columns: htmlColumns,
          data: [{ id: 1, name: 'Bold', formatted: 'test' }],
        },
      });

      // HTML should be rendered
      const strongElement = document.querySelector('strong');
      expect(strongElement).toBeInTheDocument();
      expect(strongElement.textContent).toBe('Bold');
    });
  });

  describe('Row Clicks', () => {
    it('should call onRowClick when row is clicked', async () => {
      const mockOnRowClick = vi.fn();

      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
          onRowClick: mockOnRowClick,
        },
      });

      const firstRow = document.querySelector('tbody tr');
      await fireEvent.click(firstRow);

      expect(mockOnRowClick).toHaveBeenCalledTimes(1);
      expect(mockOnRowClick).toHaveBeenCalledWith(sampleData[0]);
    });

    it('should apply cursor-pointer class when onRowClick is provided', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
          onRowClick: vi.fn(),
        },
      });

      const rows = document.querySelectorAll('tbody tr');
      rows.forEach((row) => {
        expect(row.classList.contains('cursor-pointer')).toBe(true);
      });
    });

    it('should not apply cursor-pointer when onRowClick is not provided', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData,
        },
      });

      const rows = document.querySelectorAll('tbody tr');
      rows.forEach((row) => {
        expect(row.classList.contains('cursor-pointer')).toBe(false);
      });
    });
  });

  describe('Pagination', () => {
    const largeData = Array.from({ length: 50 }, (_, i) => ({
      id: i + 1,
      name: `User ${i + 1}`,
      email: `user${i + 1}@example.com`,
      status: i % 2 === 0 ? 'Active' : 'Inactive',
    }));

    it('should display correct items per page', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: largeData,
          pagination: true,
          pageSize: 10,
        },
      });

      const rows = document.querySelectorAll('tbody tr');
      expect(rows.length).toBe(10);
    });

    it('should show pagination info correctly', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: largeData,
          pagination: true,
          pageSize: 10,
        },
      });

      expect(screen.getByText(/Showing 1–10 of 50/)).toBeInTheDocument();
    });

    it('should show correct page count', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: largeData,
          pagination: true,
          pageSize: 10,
        },
      });

      expect(screen.getByText(/Page 1 of 5/)).toBeInTheDocument();
    });

    it('should not show pagination when data fits on one page', async () => {
      render(DataTable, {
        props: {
          columns: sampleColumns,
          data: sampleData, // 3 items
          pagination: true,
          pageSize: 25,
        },
      });

      // Pagination controls should not be visible
      expect(screen.queryByText(/Page 1 of/)).not.toBeInTheDocument();
    });
  });

  describe('Nested Property Access', () => {
    it('should access nested properties using dot notation', async () => {
      const nestedColumns = [
        { key: 'user.name', label: 'User Name' },
        { key: 'user.email', label: 'User Email' },
      ];

      const nestedData = [
        { id: 1, user: { name: 'John Doe', email: 'john@example.com' } },
        { id: 2, user: { name: 'Jane Doe', email: 'jane@example.com' } },
      ];

      render(DataTable, {
        props: {
          columns: nestedColumns,
          data: nestedData,
        },
      });

      expect(screen.getByText('John Doe')).toBeInTheDocument();
      expect(screen.getByText('john@example.com')).toBeInTheDocument();
    });

    it('should handle missing nested properties gracefully', async () => {
      const nestedColumns = [{ key: 'user.profile.avatar', label: 'Avatar' }];

      const incompleteData = [
        { id: 1, user: { name: 'John' } }, // Missing profile.avatar
      ];

      render(DataTable, {
        props: {
          columns: nestedColumns,
          data: incompleteData,
        },
      });

      // Should show fallback dash
      expect(screen.getByText('—')).toBeInTheDocument();
    });
  });

  describe('Column Configuration', () => {
    it('should apply custom column width', async () => {
      const widthColumns = [
        { key: 'name', label: 'Name', width: '200px' },
        { key: 'email', label: 'Email' },
      ];

      render(DataTable, {
        props: {
          columns: widthColumns,
          data: sampleData,
        },
      });

      const headerCell = document.querySelector('thead th');
      expect(headerCell.style.width).toBe('200px');
    });

    it('should apply column alignment', async () => {
      const alignColumns = [
        { key: 'name', label: 'Name', align: 'text-center' },
        { key: 'amount', label: 'Amount', align: 'text-right' },
      ];

      render(DataTable, {
        props: {
          columns: alignColumns,
          data: [{ id: 1, name: 'Test', amount: 100 }],
        },
      });

      const headerCells = document.querySelectorAll('thead th');
      expect(headerCells[0].classList.contains('text-center')).toBe(true);
      expect(headerCells[1].classList.contains('text-right')).toBe(true);
    });
  });
});
