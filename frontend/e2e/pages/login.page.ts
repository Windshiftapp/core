import { expect, type Page } from '@playwright/test';

/**
 * Page Object for the Login Dialog
 */
export class LoginPage {
  constructor(private page: Page) {}

  // Selectors
  readonly loginDialog =
    'div[role="dialog"]:has(input[type="password"]), .login-modal, .login-dialog';
  readonly usernameInput =
    'input[type="text"]:not([type="email"]), input[name="username"], input[placeholder*="username"]';
  readonly passwordInput = 'input[type="password"]';
  readonly rememberMeCheckbox =
    'input[type="checkbox"][name="remember"], label:has-text("Remember me")';
  readonly loginButton =
    'button[type="submit"], button:has-text("Login"), button:has-text("Sign in")';
  readonly errorMessage = '.error, .error-message, [role="alert"]';
  readonly forgotPasswordLink = 'a:has-text("Forgot password"), button:has-text("Forgot password")';

  /**
   * Navigate to the login page
   */
  async goto() {
    await this.page.goto('/');
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Check if login dialog is visible
   */
  async isLoginDialogVisible(): Promise<boolean> {
    try {
      // First wait for either password input or the dialog itself
      await this.page.waitForSelector(`${this.passwordInput}, ${this.loginDialog}`, {
        timeout: 5000,
      });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Fill in username
   */
  async fillUsername(username: string) {
    await this.page.fill(this.usernameInput, username);
  }

  /**
   * Fill in password
   */
  async fillPassword(password: string) {
    await this.page.fill(this.passwordInput, password);
  }

  /**
   * Toggle remember me checkbox
   */
  async setRememberMe(checked: boolean) {
    const checkbox = this.page.locator(this.rememberMeCheckbox).first();
    const isChecked = await checkbox.isChecked().catch(() => false);
    if (isChecked !== checked) {
      await checkbox.check();
    }
  }

  /**
   * Click login button
   */
  async clickLogin() {
    await this.page.click(this.loginButton);
  }

  /**
   * Perform complete login
   */
  async login(username: string, password: string, rememberMe: boolean = false) {
    await this.goto();

    // Wait for login dialog to appear
    const isVisible = await this.isLoginDialogVisible();
    if (!isVisible) {
      throw new Error('Login dialog did not appear');
    }

    await this.fillUsername(username);
    await this.fillPassword(password);

    if (rememberMe) {
      await this.setRememberMe(true);
    }

    await this.clickLogin();

    // Wait for login to complete
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify successful login
   */
  async verifyLoginSuccess() {
    // After successful login, login dialog should disappear
    // and we should have a session cookie
    const cookies = await this.page.context().cookies();
    const hasSession = cookies.some((c) => c.name === 'session' || c.name === 'windshift_session');
    expect(hasSession).toBeTruthy();

    // Login dialog should be gone
    await expect(this.page.locator(this.loginDialog)).not.toBeVisible({ timeout: 10000 });
  }

  /**
   * Verify login failed
   */
  async verifyLoginFailed() {
    // Error message should appear
    await expect(this.page.locator(this.errorMessage)).toBeVisible({ timeout: 5000 });
  }

  /**
   * Get error message text
   */
  async getErrorMessage(): Promise<string> {
    const errorElement = this.page.locator(this.errorMessage).first();
    return (await errorElement.textContent()) || '';
  }

  /**
   * Logout (via user menu)
   */
  async logout() {
    // Click user menu button/avatar
    await this.page.click('[data-testid="user-menu"], .user-avatar, button:has-text("admin")');

    // Wait for menu to appear
    await this.page.waitForSelector('text=Logout, text=Sign out', { timeout: 5000 });

    // Click logout
    await this.page.click('text=Logout, text=Sign out');

    // Wait for logout to complete
    await this.page.waitForTimeout(1000);
  }

  /**
   * Verify logout success
   */
  async verifyLogoutSuccess() {
    // After logout, login dialog should appear again
    const isVisible = await this.isLoginDialogVisible();
    expect(isVisible).toBeTruthy();
  }
}
