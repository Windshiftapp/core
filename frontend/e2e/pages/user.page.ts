import { expect, type Page } from '@playwright/test';

/**
 * Page Object for User Management (Admin)
 */
export class UserPage {
  constructor(private page: Page) {}

  // Selectors
  readonly adminLink = 'a:has-text("Admin"), nav a[href="/admin"]';
  readonly usersTab = 'button:has-text("Users"), a:has-text("Users")';
  readonly createUserButton =
    'button:has-text("Create User"), button:has-text("Add User"), button:has-text("New User")';
  readonly userModal = 'div[role="dialog"], .modal, .user-modal';
  readonly emailInput = 'input[name="email"], input[type="email"]';
  readonly usernameInput = 'input[name="username"]';
  readonly firstNameInput = 'input[name="first_name"], input[name="firstName"]';
  readonly lastNameInput = 'input[name="last_name"], input[name="lastName"]';
  readonly passwordInput = 'input[name="password"], input[type="password"]';
  readonly isActiveCheckbox =
    'input[name="is_active"], input[type="checkbox"]:near(label:has-text("Active"))';
  readonly saveButton = 'button[type="submit"], button:has-text("Save"), button:has-text("Create")';
  readonly cancelButton = 'button:has-text("Cancel"), button:has-text("Close")';
  readonly userRow = '.user-row, tr, [data-testid="user-row"]';
  readonly editButton = 'button:has-text("Edit"), [aria-label="Edit"]';
  readonly deleteButton = 'button:has-text("Delete"), [aria-label="Delete"]';
  readonly deactivateButton = 'button:has-text("Deactivate")';
  readonly activateButton = 'button:has-text("Activate")';
  readonly confirmButton = 'button:has-text("Confirm"), button:has-text("Yes")';
  readonly permissionsTab = 'button:has-text("Permissions"), a:has-text("Permissions")';
  readonly roleSelect = 'select[name="role"], [name="role"]';
  readonly permissionCheckbox = 'input[type="checkbox"]';

  /**
   * Navigate to admin users page
   */
  async goto() {
    await this.page.goto('/admin');
    await this.page.waitForLoadState('networkidle');

    // Click on Users tab
    try {
      await this.page.click(this.usersTab);
      await this.page.waitForTimeout(500);
    } catch {
      // Tab might not exist or already selected
    }
  }

  /**
   * Click create user button
   */
  async clickCreate() {
    await this.page.click(this.createUserButton);
    await this.page.waitForSelector(this.userModal, { timeout: 5000 });
  }

  /**
   * Fill user form
   */
  async fillForm(data: {
    email: string;
    username: string;
    firstName: string;
    lastName: string;
    password?: string;
    isActive?: boolean;
  }) {
    await this.page.fill(this.emailInput, data.email);
    await this.page.fill(this.usernameInput, data.username);
    await this.page.fill(this.firstNameInput, data.firstName);
    await this.page.fill(this.lastNameInput, data.lastName);

    if (data.password) {
      await this.page.fill(this.passwordInput, data.password);
    }

    if (data.isActive !== undefined) {
      const checkbox = this.page.locator(this.isActiveCheckbox);
      const isChecked = await checkbox.isChecked().catch(() => true);
      if (isChecked !== data.isActive) {
        await checkbox.click();
      }
    }
  }

  /**
   * Click save button
   */
  async clickSave() {
    await this.page.click(this.saveButton);
    await this.page.waitForTimeout(1000);
  }

  /**
   * Create a new user
   */
  async createUser(data: {
    email: string;
    username: string;
    firstName: string;
    lastName: string;
    password: string;
    isActive?: boolean;
  }) {
    await this.goto();
    await this.clickCreate();
    await this.fillForm(data);
    await this.clickSave();

    // Wait for creation
    await this.page.waitForTimeout(2000);
  }

  /**
   * Find user by username
   */
  async findUserByUsername(username: string) {
    return this.page.locator(`${this.userRow}:has-text("${username}")`).first();
  }

  /**
   * Find user by email
   */
  async findUserByEmail(email: string) {
    return this.page.locator(`${this.userRow}:has-text("${email}")`).first();
  }

  /**
   * Verify user exists
   */
  async verifyUserExists(username: string) {
    await this.goto();
    const user = await this.findUserByUsername(username);
    await expect(user).toBeVisible({ timeout: 10000 });
  }

  /**
   * Edit a user
   */
  async editUser(
    username: string,
    newData: {
      email?: string;
      firstName?: string;
      lastName?: string;
      isActive?: boolean;
    }
  ) {
    await this.goto();

    // Find and click edit button
    const user = await this.findUserByUsername(username);
    await user.locator(this.editButton).click();

    // Wait for modal
    await this.page.waitForSelector(this.userModal, { timeout: 5000 });

    // Update fields
    if (newData.email) {
      await this.page.fill(this.emailInput, newData.email);
    }
    if (newData.firstName) {
      await this.page.fill(this.firstNameInput, newData.firstName);
    }
    if (newData.lastName) {
      await this.page.fill(this.lastNameInput, newData.lastName);
    }
    if (newData.isActive !== undefined) {
      const checkbox = this.page.locator(this.isActiveCheckbox);
      const isChecked = await checkbox.isChecked().catch(() => true);
      if (isChecked !== newData.isActive) {
        await checkbox.click();
      }
    }

    await this.clickSave();

    // Wait for update
    await this.page.waitForTimeout(2000);
  }

  /**
   * Deactivate a user
   */
  async deactivateUser(username: string) {
    await this.goto();

    // Find and click deactivate button
    const user = await this.findUserByUsername(username);
    await user.locator(this.deactivateButton).click();

    // Confirm
    await this.page.waitForSelector(this.confirmButton, { timeout: 5000 });
    await this.page.click(this.confirmButton);

    // Wait for deactivation
    await this.page.waitForTimeout(2000);
  }

  /**
   * Activate a user
   */
  async activateUser(username: string) {
    await this.goto();

    // Find and click activate button
    const user = await this.findUserByUsername(username);
    await user.locator(this.activateButton).click();

    // Confirm
    await this.page.waitForSelector(this.confirmButton, { timeout: 5000 });
    await this.page.click(this.confirmButton);

    // Wait for activation
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify user is active
   */
  async verifyUserIsActive(username: string) {
    await this.goto();
    const user = await this.findUserByUsername(username);
    const statusBadge = user.locator('.status, .badge, [data-status="active"]');
    await expect(statusBadge).toContainText('Active');
  }

  /**
   * Verify user is inactive
   */
  async verifyUserIsInactive(username: string) {
    await this.goto();
    const user = await this.findUserByUsername(username);
    const statusBadge = user.locator('.status, .badge, [data-status="inactive"]');
    await expect(statusBadge).toContainText('Inactive');
  }

  /**
   * Assign permission to user
   */
  async assignPermission(username: string, permission: string) {
    await this.goto();

    // Click on user
    const user = await this.findUserByUsername(username);
    await user.click();

    // Go to permissions tab
    await this.page.click(this.permissionsTab);

    // Find and check permission checkbox
    const permissionCheckbox = this.page.locator(
      `${this.permissionCheckbox}:near(label:has-text("${permission}"))`
    );
    await permissionCheckbox.check();

    // Save
    await this.clickSave();

    // Wait for save
    await this.page.waitForTimeout(1000);
  }

  /**
   * Verify user has permission
   */
  async verifyUserHasPermission(username: string, permission: string) {
    await this.goto();

    // Click on user
    const user = await this.findUserByUsername(username);
    await user.click();

    // Go to permissions tab
    await this.page.click(this.permissionsTab);

    // Verify permission is checked
    const permissionCheckbox = this.page.locator(
      `${this.permissionCheckbox}:near(label:has-text("${permission}"))`
    );
    await expect(permissionCheckbox).toBeChecked();
  }

  /**
   * Get user count
   */
  async getUserCount(): Promise<number> {
    await this.goto();
    const users = await this.page.locator(this.userRow).count();
    return users;
  }

  /**
   * Search for user
   */
  async searchUser(query: string) {
    const searchInput = this.page.locator('input[type="search"], input[placeholder*="Search"]');
    await searchInput.fill(query);
    await this.page.waitForTimeout(500);
  }
}
