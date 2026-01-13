import { Page, expect } from '@playwright/test';

/**
 * Page Object for the Welcome Assistant (Initial Setup)
 */
export class SetupPage {
  constructor(private page: Page) {}

  // Selectors
  readonly setupModal = 'div[role="dialog"], .modal';
  readonly emailInput = 'input[type="email"], input[name="email"]';
  readonly usernameInput = 'input[name="username"], input[placeholder*="username"]';
  readonly passwordInput = 'input[type="password"], input[name="password"]';
  readonly firstNameInput = 'input[name="first_name"], input[name="firstName"]';
  readonly lastNameInput = 'input[name="last_name"], input[name="lastName"]';
  readonly timeTrackingCheckbox = 'input[type="checkbox"][name="time_tracking"], label:has-text("Time Tracking")';
  readonly testManagementCheckbox = 'input[type="checkbox"][name="test_management"], label:has-text("Test Management")';
  readonly submitButton = 'button[type="submit"], button:has-text("Complete Setup"), button:has-text("Get Started")';
  readonly successMessage = 'text=Setup completed, text=Welcome';

  /**
   * Navigate to the setup page
   */
  async goto() {
    await this.page.goto('/');
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Check if setup modal is visible
   */
  async isSetupModalVisible(): Promise<boolean> {
    try {
      await this.page.waitForSelector(this.setupModal, { timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Fill in the admin user form
   */
  async fillAdminUserForm(data: {
    email: string;
    username: string;
    password: string;
    firstName: string;
    lastName: string;
  }) {
    await this.page.fill(this.emailInput, data.email);
    await this.page.fill(this.usernameInput, data.username);
    await this.page.fill(this.passwordInput, data.password);
    await this.page.fill(this.firstNameInput, data.firstName);
    await this.page.fill(this.lastNameInput, data.lastName);
  }

  /**
   * Configure module settings
   */
  async configureModules(options: {
    timeTracking?: boolean;
    testManagement?: boolean;
  }) {
    if (options.timeTracking !== undefined) {
      const checkbox = this.page.locator(this.timeTrackingCheckbox);
      const isChecked = await checkbox.isChecked().catch(() => false);
      if (isChecked !== options.timeTracking) {
        await checkbox.check();
      }
    }

    if (options.testManagement !== undefined) {
      const checkbox = this.page.locator(this.testManagementCheckbox);
      const isChecked = await checkbox.isChecked().catch(() => false);
      if (isChecked !== options.testManagement) {
        await checkbox.check();
      }
    }
  }

  /**
   * Submit the setup form
   */
  async submit() {
    await this.page.click(this.submitButton);
  }

  /**
   * Complete full setup process
   */
  async completeSetup(data: {
    email: string;
    username: string;
    password: string;
    firstName: string;
    lastName: string;
    timeTracking?: boolean;
    testManagement?: boolean;
  }) {
    await this.goto();

    const isVisible = await this.isSetupModalVisible();
    if (!isVisible) {
      console.log('Setup already completed');
      return;
    }

    await this.fillAdminUserForm({
      email: data.email,
      username: data.username,
      password: data.password,
      firstName: data.firstName,
      lastName: data.lastName,
    });

    await this.configureModules({
      timeTracking: data.timeTracking,
      testManagement: data.testManagement,
    });

    await this.submit();

    // Wait for setup to complete
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify setup completion
   */
  async verifySetupCompleted() {
    // After setup, we should either see success message or be redirected
    // The setup modal should be gone
    await expect(this.page.locator(this.setupModal)).not.toBeVisible({ timeout: 10000 });
  }
}
