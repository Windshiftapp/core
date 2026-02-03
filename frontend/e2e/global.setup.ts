import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test as setup } from '@playwright/test';

/**
 * Global setup that runs once before all tests
 * Completes application setup and creates authenticated session
 */

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const authFile = path.join(__dirname, '../.auth/user.json');

setup('complete application setup and authenticate', async ({ page, request }) => {
  const baseURL = process.env.BASE_URL || 'http://localhost:8080';

  // Step 1: Check if setup is already completed
  const statusResponse = await request.get(`${baseURL}/api/setup/status`);
  expect(statusResponse.ok()).toBeTruthy();

  const setupStatus = await statusResponse.json();

  if (!setupStatus.setup_completed) {
    console.log('🔧 Application setup not completed, running initial setup...');

    // Get CSRF token for setup
    const csrfResponse = await request.get(`${baseURL}/api/csrf-token`);
    const csrfData = await csrfResponse.json();
    const csrfToken = csrfData.csrf_token;

    // Complete initial setup
    const setupResponse = await request.post(`${baseURL}/api/setup/complete`, {
      headers: {
        'X-CSRF-Token': csrfToken,
      },
      data: {
        admin_user: {
          email: 'admin@e2etest.com',
          username: 'admin',
          password_hash: 'TestPass123!', // Will be hashed server-side
          first_name: 'E2E',
          last_name: 'Admin',
        },
        module_settings: {
          time_tracking_enabled: true,
          test_management_enabled: true,
        },
      },
    });

    expect(setupResponse.ok()).toBeTruthy();
    console.log('✅ Application setup completed successfully');
  } else {
    console.log('✅ Application setup already completed');
  }

  // Step 2: Login to get session
  console.log('🔐 Logging in to create authenticated session...');

  await page.goto(`${baseURL}/`);

  // Wait for login dialog to appear (it appears after setup check completes)
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });

  // Fill in login credentials
  await page.fill('input[type="text"]', 'admin');
  await page.fill('input[type="password"]', 'TestPass123!');

  // Click login button
  await page.click('button[type="submit"]');

  // Wait for successful login (URL should change or login dialog should close)
  await page.waitForTimeout(2000);

  // Verify we're logged in by checking for auth cookie or redirect
  const cookies = await page.context().cookies();
  const hasSessionCookie = cookies.some(
    (cookie) => cookie.name === 'session' || cookie.name === 'windshift_session'
  );

  expect(hasSessionCookie).toBeTruthy();
  console.log('✅ Authentication successful');

  // Step 3: Save storage state for reuse
  await page.context().storageState({ path: authFile });
  console.log('💾 Authentication state saved for reuse');
});
