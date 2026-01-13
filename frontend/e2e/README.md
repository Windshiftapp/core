# Windshift E2E Tests

Comprehensive end-to-end tests for Windshift using Playwright. These tests validate the application from a user's perspective, testing real browser interactions across multiple browsers.

## Overview

The E2E test suite includes:

- **Setup Tests**: Initial application setup and configuration
- **Authentication Tests**: Login, logout, and session management
- **Workspace Tests**: Workspace CRUD operations
- **Item Tests**: Work item management and hierarchy
- **User Tests**: User management and permissions

## Prerequisites

1. **Node.js** (v18 or higher)
2. **Windshift Server** running on `http://localhost:8080` (or custom BASE_URL)
3. **Fresh Database** for first run (tests will complete setup automatically)

## Installation

```bash
cd frontend

# Install dependencies
npm install

# Install Playwright browsers
npx playwright install
```

## Running Tests

### All Tests

```bash
# Run all tests across all browsers
npm run test:e2e

# Run in headed mode (see browser)
npm run test:e2e:headed

# Run in debug mode (interactive debugging)
npm run test:e2e:debug
```

### Specific Test Files

```bash
# Run setup tests only
npx playwright test tests/01-setup.spec.ts

# Run authentication tests
npx playwright test tests/02-authentication.spec.ts

# Run workspace tests
npx playwright test tests/03-workspaces.spec.ts

# Run item tests
npx playwright test tests/04-items.spec.ts

# Run user tests
npx playwright test tests/06-users.spec.ts
```

### Specific Browsers

```bash
# Run in Chromium only
npx playwright test --project=chromium

# Run in Firefox only
npx playwright test --project=firefox

# Run in WebKit only
npx playwright test --project=webkit
```

### Other Options

```bash
# Run tests matching a pattern
npx playwright test -g "should create workspace"

# Run with UI mode (interactive)
npx playwright test --ui

# Run with trace
npx playwright test --trace on

# Generate HTML report
npm run test:e2e:report
```

## Test Structure

```
e2e/
├── fixtures/           # Test fixtures and helpers
│   ├── auth.ts        # Authentication fixtures
│   └── test-data.ts   # Test data generators
├── pages/             # Page Object Models
│   ├── setup.page.ts
│   ├── login.page.ts
│   ├── workspace.page.ts
│   ├── item.page.ts
│   └── user.page.ts
├── tests/             # Test suites
│   ├── 01-setup.spec.ts
│   ├── 02-authentication.spec.ts
│   ├── 03-workspaces.spec.ts
│   ├── 04-items.spec.ts
│   └── 06-users.spec.ts
├── global.setup.ts    # Global setup (runs once)
└── README.md          # This file
```

## Configuration

### Environment Variables

```bash
# Set custom base URL
BASE_URL=http://localhost:3000 npm run test:e2e

# Run in CI mode
CI=true npm run test:e2e
```

### Playwright Config

Edit `playwright.config.ts` to customize:

- Base URL
- Timeouts
- Browsers
- Parallelization
- Screenshots/Videos
- Reporters

## Test Execution Flow

1. **Global Setup** (`global.setup.ts`):
   - Completes application setup if needed
   - Creates admin user
   - Logs in and saves authentication state

2. **Tests Run**:
   - Each test file runs with authenticated state
   - Tests use Page Object Models for interactions
   - Test data generators ensure unique identifiers

3. **Cleanup**:
   - Tests create isolated test data
   - Each test cleans up after itself where possible

## Page Object Model

Tests use the Page Object Model (POM) for maintainability:

```typescript
import { WorkspacePage } from '../pages/workspace.page';
import { generateWorkspace } from '../fixtures/test-data';

test('should create workspace', async ({ page }) => {
  const workspacePage = new WorkspacePage(page);
  const workspace = generateWorkspace();

  await workspacePage.createWorkspace(workspace);
  await workspacePage.verifyWorkspaceExists(workspace.name);
});
```

## Fixtures

### Test Data Generators

```typescript
import { generateWorkspace, generateItem, generateUser } from '../fixtures/test-data';

// Generate unique workspace
const workspace = generateWorkspace();

// Generate unique item
const item = generateItem(workspaceId, 'suffix');

// Generate unique user
const user = generateUser();
```

### Authentication Fixtures

```typescript
import { test } from '../fixtures/auth';

test('should make authenticated API call', async ({ makeAuthRequest, request }) => {
  const token = 'your-bearer-token';
  const response = await makeAuthRequest(request, token, 'GET', '/workspaces');
  // ...
});
```

## Debugging

### Visual Debugging

```bash
# Run in headed mode
npx playwright test --headed

# Run in debug mode (step through tests)
npx playwright test --debug

# Run with UI mode (interactive)
npx playwright test --ui
```

### Traces

```bash
# Capture trace on failure
npx playwright test --trace on-first-retry

# View trace
npx playwright show-trace trace.zip
```

### Screenshots

Screenshots are automatically captured on failure and saved to `test-results/`.

### Videos

Videos are recorded on failure and saved to `test-results/`.

## Troubleshooting

### Server Not Running

Ensure the Windshift server is running:

```bash
cd /Users/stefanernst/realigned/windshift
./windshift -db e2e-test.db -p 8080
```

### Setup Already Completed

If setup tests fail because setup is already done:
- Use a fresh database
- Or skip setup tests: `npx playwright test --grep-invert "Setup"`

### Test Failures

1. Check server logs for errors
2. View screenshots in `test-results/`
3. Run in headed mode to watch: `npx playwright test --headed`
4. Enable trace: `npx playwright test --trace on`

### Port Conflicts

If port 8080 is in use, set custom BASE_URL:

```bash
BASE_URL=http://localhost:3000 npm run test:e2e
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: 18

      - name: Install dependencies
        run: |
          cd frontend
          npm install
          npx playwright install --with-deps

      - name: Build frontend
        run: |
          cd frontend
          npm run build

      - name: Start server
        run: |
          ./windshift -db e2e-test.db -p 8080 &
          sleep 5

      - name: Run E2E tests
        run: |
          cd frontend
          npm run test:e2e

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: playwright-report
          path: frontend/playwright-report/
```

## Best Practices

1. **Use Page Objects**: Encapsulate page interactions in page objects
2. **Generate Test Data**: Use test data generators for unique identifiers
3. **Isolated Tests**: Each test should be independent
4. **Cleanup**: Tests should clean up created data where possible
5. **Assertions**: Use explicit assertions with timeout
6. **Wait Strategies**: Use `waitForLoadState`, not arbitrary timeouts
7. **Selectors**: Prefer data-testid attributes for stable selectors

## Writing New Tests

1. Create page object in `pages/` if needed
2. Add test file in `tests/` with sequential number
3. Import required page objects and fixtures
4. Write tests using Page Object Model
5. Run tests to verify

Example:

```typescript
import { test, expect } from '@playwright/test';
import { MyPage } from '../pages/my.page';
import { generateMyData } from '../fixtures/test-data';

test.describe('My Feature', () => {
  let myPage: MyPage;

  test.beforeEach(async ({ page }) => {
    myPage = new MyPage(page);
  });

  test('should do something', async () => {
    const data = generateMyData();

    await myPage.doSomething(data);
    await myPage.verifyResult();
  });
});
```

## Additional Resources

- [Playwright Documentation](https://playwright.dev)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)
- [Page Object Model](https://playwright.dev/docs/pom)
- [Debugging Tests](https://playwright.dev/docs/debug)

## Support

For issues or questions:
- Check test output and screenshots
- Review Playwright documentation
- Check server logs for backend errors
- Run tests in debug mode for interactive troubleshooting
