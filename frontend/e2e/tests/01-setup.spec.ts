import { expect, test } from '@playwright/test';
import { generateUser } from '../fixtures/test-data';
import { SetupPage } from '../pages/setup.page';

/**
 * Initial Setup Tests
 * These tests verify the application setup flow
 * Note: These tests should run on a fresh database without global setup
 */

test.describe('Application Setup', () => {
  test.use({ storageState: { cookies: [], origins: [] } }); // No auth state

  test('should display setup wizard on first launch', async ({ page, request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Check setup status
    const statusResponse = await request.get(`${baseURL}/api/setup/status`);
    const setupStatus = await statusResponse.json();

    // If setup is already completed, skip this test
    if (setupStatus.setup_completed) {
      test.skip();
      return;
    }

    // Navigate to app
    await page.goto(baseURL);

    // Setup wizard should appear
    const setupPage = new SetupPage(page);
    const isVisible = await setupPage.isSetupModalVisible();
    expect(isVisible).toBeTruthy();
  });

  test('should validate required fields', async ({ page, request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Check setup status
    const statusResponse = await request.get(`${baseURL}/api/setup/status`);
    const setupStatus = await statusResponse.json();

    if (setupStatus.setup_completed) {
      test.skip();
      return;
    }

    const setupPage = new SetupPage(page);
    await setupPage.goto();

    // Wait for setup modal
    const isVisible = await setupPage.isSetupModalVisible();
    if (!isVisible) {
      test.skip();
      return;
    }

    // Try to submit without filling required fields
    await setupPage.submit();

    // Should see validation errors or form should not submit
    await page.waitForTimeout(1000);

    // Setup modal should still be visible
    const stillVisible = await setupPage.isSetupModalVisible();
    expect(stillVisible).toBeTruthy();
  });

  test('should complete setup with valid data', async ({ page, request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Check setup status
    const statusResponse = await request.get(`${baseURL}/api/setup/status`);
    const setupStatus = await statusResponse.json();

    if (setupStatus.setup_completed) {
      test.skip();
      return;
    }

    const setupPage = new SetupPage(page);
    const adminUser = generateUser('setup-test');

    await setupPage.completeSetup({
      email: adminUser.email,
      username: adminUser.username,
      password: adminUser.password_hash,
      firstName: adminUser.first_name,
      lastName: adminUser.last_name,
      timeTracking: true,
      testManagement: true,
    });

    // Verify setup completed
    await setupPage.verifySetupCompleted();

    // Verify setup status via API
    const newStatusResponse = await request.get(`${baseURL}/api/setup/status`);
    const newSetupStatus = await newStatusResponse.json();
    expect(newSetupStatus.setup_completed).toBeTruthy();
  });

  test('should not allow setup after completion', async ({ page, request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Check setup status
    const statusResponse = await request.get(`${baseURL}/api/setup/status`);
    const setupStatus = await statusResponse.json();

    if (!setupStatus.setup_completed) {
      test.skip();
      return;
    }

    // Try to complete setup again via API
    const csrfResponse = await request.get(`${baseURL}/api/csrf-token`);
    const csrfData = await csrfResponse.json();

    const adminUser = generateUser('duplicate-setup');

    const setupResponse = await request.post(`${baseURL}/api/setup/complete`, {
      headers: {
        'X-CSRF-Token': csrfData.csrf_token,
      },
      data: {
        admin_user: {
          email: adminUser.email,
          username: adminUser.username,
          password_hash: adminUser.password_hash,
          first_name: adminUser.first_name,
          last_name: adminUser.last_name,
        },
        module_settings: {
          time_tracking_enabled: true,
          test_management_enabled: true,
        },
      },
    });

    // Should fail (400 Bad Request or similar)
    expect(setupResponse.status()).toBe(400);
  });

  test('should have created admin user after setup', async ({ page, request }) => {
    const baseURL = process.env.BASE_URL || 'http://localhost:8080';

    // Check setup status
    const statusResponse = await request.get(`${baseURL}/api/setup/status`);
    const setupStatus = await statusResponse.json();

    if (!setupStatus.setup_completed) {
      test.skip();
      return;
    }

    // Verify admin user can login
    await page.goto(baseURL);

    // Login dialog should appear
    await page.waitForSelector('input[type="password"]', { timeout: 10000 });

    // Try to login with admin credentials
    await page.fill('input[type="text"]', 'admin');
    await page.fill('input[type="password"]', 'TestPass123!');
    await page.click('button[type="submit"]');

    // Wait for login
    await page.waitForTimeout(2000);

    // Should have session cookie
    const cookies = await page.context().cookies();
    const hasSession = cookies.some((c) => c.name === 'session' || c.name === 'windshift_session');
    expect(hasSession).toBeTruthy();
  });
});
