import '@testing-library/jest-dom/vitest';

// Mock CSS variables used by components
const mockCSSVariables = `
  :root {
    --ds-surface: #ffffff;
    --ds-surface-raised: #ffffff;
    --ds-surface-overlay: #ffffff;
    --ds-text: #1a1a1a;
    --ds-text-subtle: #6b7280;
    --ds-text-disabled: #9ca3af;
    --ds-border: #e5e7eb;
    --ds-border-bold: #d1d5db;
    --ds-background-neutral: #f3f4f6;
    --ds-background-neutral-hovered: #e5e7eb;
  }
`;

// Inject mock CSS variables into jsdom
const style = document.createElement('style');
style.textContent = mockCSSVariables;
document.head.appendChild(style);

// Mock window.matchMedia for components that use media queries
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Reset mocks between tests
beforeEach(() => {
  vi.clearAllMocks();
});
