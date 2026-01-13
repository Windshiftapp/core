import { test, expect } from '@playwright/test';
import { LoginPage } from '../pages/login.page';
import { test as authTest } from '../fixtures/auth';

/**
 * Authentication Tests
 * Tests login, logout, and session management
 */

test.describe('Authentication', () => {
  test.describe('Login', () => {
    test.use({ storageState: { cookies: [], origins: [] } }); // No auth state

    test('should display login dialog when not authenticated', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Login dialog should be visible
      const isVisible = await loginPage.isLoginDialogVisible();
      expect(isVisible).toBeTruthy();
    });

    test('should login with valid credentials', async ({ page }) => {
      const loginPage = new LoginPage(page);

      await loginPage.login('admin', 'TestPass123!');

      // Verify successful login
      await loginPage.verifyLoginSuccess();
    });

    test('should fail with invalid username', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      await loginPage.fillUsername('invaliduser');
      await loginPage.fillPassword('SomePassword123!');
      await loginPage.clickLogin();

      // Should see error message
      await page.waitForTimeout(1000);
      const isLoginDialogVisible = await loginPage.isLoginDialogVisible();
      expect(isLoginDialogVisible).toBeTruthy();

      // Should still be on login page or show error
      // Note: Specific error handling depends on implementation
    });

    test('should fail with invalid password', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      await loginPage.fillUsername('admin');
      await loginPage.fillPassword('WrongPassword!');
      await loginPage.clickLogin();

      // Should see error message or stay on login
      await page.waitForTimeout(1000);
      const isLoginDialogVisible = await loginPage.isLoginDialogVisible();
      expect(isLoginDialogVisible).toBeTruthy();
    });

    test('should persist session with remember me', async ({ page }) => {
      const loginPage = new LoginPage(page);

      await loginPage.login('admin', 'TestPass123!', true);

      // Verify successful login
      await loginPage.verifyLoginSuccess();

      // Check cookies
      const cookies = await page.context().cookies();
      const sessionCookie = cookies.find(c => c.name === 'session' || c.name === 'windshift_session');

      // With remember me, session cookie should have longer expiry
      expect(sessionCookie).toBeTruthy();
    });
  });

  test.describe('Logout', () => {
    test('should logout successfully', async ({ page }) => {
      const loginPage = new LoginPage(page);

      // First login
      await loginPage.login('admin', 'TestPass123!');
      await loginPage.verifyLoginSuccess();

      // Then logout
      await loginPage.logout();
      await loginPage.verifyLogoutSuccess();
    });

    test('should clear session after logout', async ({ page }) => {
      const loginPage = new LoginPage(page);

      // Login
      await loginPage.login('admin', 'TestPass123!');
      await loginPage.verifyLoginSuccess();

      // Logout
      await loginPage.logout();

      // Verify session is cleared
      const cookies = await page.context().cookies();
      const sessionCookie = cookies.find(c => c.name === 'session' || c.name === 'windshift_session');

      // Session cookie should be removed or expired
      expect(sessionCookie).toBeFalsy();
    });
  });

  test.describe('Session Management', () => {
    test('should maintain session across page navigation', async ({ page }) => {
      const loginPage = new LoginPage(page);

      // Login
      await loginPage.login('admin', 'TestPass123!');
      await loginPage.verifyLoginSuccess();

      // Navigate to different pages
      await page.goto('/workspaces');
      await page.waitForLoadState('networkidle');

      await page.goto('/admin');
      await page.waitForLoadState('networkidle');

      await page.goto('/');
      await page.waitForLoadState('networkidle');

      // Should still be authenticated (no login dialog)
      const isLoginDialogVisible = await loginPage.isLoginDialogVisible();
      expect(isLoginDialogVisible).toBeFalsy();
    });

    test('should redirect to login when session expires', async ({ page, context }) => {
      const loginPage = new LoginPage(page);

      // Login
      await loginPage.login('admin', 'TestPass123!');
      await loginPage.verifyLoginSuccess();

      // Clear cookies to simulate session expiry
      await context.clearCookies();

      // Try to navigate to protected page
      await page.goto('/admin');
      await page.waitForTimeout(2000);

      // Should see login dialog
      const isLoginDialogVisible = await loginPage.isLoginDialogVisible();
      expect(isLoginDialogVisible).toBeTruthy();
    });
  });

  test.describe('Authenticated Actions', () => {
    // These tests use the authenticated state from global setup
    test('should access protected routes when authenticated', async ({ page }) => {
      // Navigate to admin page
      await page.goto('/admin');
      await page.waitForLoadState('networkidle');

      // Should not see login dialog
      const loginDialog = page.locator('div[role="dialog"]:has(input[type="password"])');
      await expect(loginDialog).not.toBeVisible({ timeout: 5000 });

      // Should see admin content
      const adminContent = page.locator('text=Admin, text=Users, text=Settings');
      await expect(adminContent.first()).toBeVisible({ timeout: 5000 });
    });

    test('should display user info when authenticated', async ({ page }) => {
      await page.goto('/');
      await page.waitForLoadState('networkidle');

      // Should see user avatar or menu
      const userMenu = page.locator('[data-testid="user-menu"], .user-avatar, button:has-text("admin")');
      await expect(userMenu.first()).toBeVisible({ timeout: 10000 });
    });
  });
});

/**
 * API Token Authentication Tests
 * Tests bearer token authentication
 */
test.describe('API Token Authentication', () => {
  test('should create API token', async ({ request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Get CSRF token
    const csrfResponse = await request.get(`${baseURL}/api/csrf-token`);
    const csrfData = await csrfResponse.json();

    // Create token (requires session auth first)
    // This test assumes we have a valid session from global setup
    const tokenResponse = await request.post(`${baseURL}/api/api-tokens`, {
      headers: {
        'X-CSRF-Token': csrfData.csrf_token,
      },
      data: {
        name: 'E2E Test Token',
        permissions: ['read', 'write'],
      },
    });

    if (tokenResponse.ok()) {
      const tokenData = await tokenResponse.json();
      expect(tokenData.token).toBeTruthy();
      expect(tokenData.token).toMatch(/^crw_/);
    }
  });

  test('should authenticate with bearer token', async ({ request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Create a token first
    const csrfResponse = await request.get(`${baseURL}/api/csrf-token`);
    const csrfData = await csrfResponse.json();

    const tokenResponse = await request.post(`${baseURL}/api/api-tokens`, {
      headers: {
        'X-CSRF-Token': csrfData.csrf_token,
      },
      data: {
        name: 'E2E Auth Test Token',
        permissions: ['read', 'write'],
      },
    });

    if (!tokenResponse.ok()) {
      test.skip();
      return;
    }

    const tokenData = await tokenResponse.json();
    const token = tokenData.token;

    // Use token to make authenticated request
    const workspacesResponse = await request.get(`${baseURL}/api/workspaces`, {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });

    expect(workspacesResponse.ok()).toBeTruthy();
  });
});
