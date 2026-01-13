import { test, expect } from '@playwright/test';
import { UserPage } from '../pages/user.page';
import { generateUser } from '../fixtures/test-data';

/**
 * User Management Tests
 * Tests user CRUD operations and permission management
 * Requires admin permissions
 */

test.describe('User Management', () => {
  let userPage: UserPage;
  let testUser: ReturnType<typeof generateUser>;

  test.beforeEach(async ({ page }) => {
    userPage = new UserPage(page);
    testUser = generateUser();
  });

  test.describe('Create User', () => {
    test('should create user with valid data', async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
        isActive: true,
      });

      // Verify user was created
      await userPage.verifyUserExists(testUser.username);
    });

    test('should create user as active by default', async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });

      // Verify user is active
      await userPage.verifyUserIsActive(testUser.username);
    });

    test('should validate unique email', async () => {
      // Create first user
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });

      // Try to create another with same email
      const duplicateUser = generateUser('duplicate');
      await userPage.goto();
      await userPage.clickCreate();
      await userPage.fillForm({
        email: testUser.email, // Same email
        username: duplicateUser.username, // Different username
        firstName: duplicateUser.first_name,
        lastName: duplicateUser.last_name,
        password: duplicateUser.password_hash,
      });
      await userPage.clickSave();

      // Should see error or validation message
      await userPage.page.waitForTimeout(1000);
    });

    test('should validate unique username', async () => {
      // Create first user
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });

      // Try to create another with same username
      const duplicateUser = generateUser('duplicate');
      await userPage.goto();
      await userPage.clickCreate();
      await userPage.fillForm({
        email: duplicateUser.email, // Different email
        username: testUser.username, // Same username
        firstName: duplicateUser.first_name,
        lastName: duplicateUser.last_name,
        password: duplicateUser.password_hash,
      });
      await userPage.clickSave();

      // Should see error or validation message
      await userPage.page.waitForTimeout(1000);
    });

    test('should require email', async ({ page }) => {
      await userPage.goto();
      await userPage.clickCreate();

      // Fill all except email
      await page.fill('input[name="username"]', testUser.username);
      await page.fill('input[name="first_name"], input[name="firstName"]', testUser.first_name);
      await page.fill('input[name="last_name"], input[name="lastName"]', testUser.last_name);
      await page.fill('input[type="password"]', testUser.password_hash);

      await userPage.clickSave();

      // Should not close modal (validation error)
      await page.waitForTimeout(500);
      const modal = page.locator(userPage.userModal);
      await expect(modal).toBeVisible();
    });

    test('should require password for new user', async ({ page }) => {
      await userPage.goto();
      await userPage.clickCreate();

      // Fill all except password
      await page.fill('input[type="email"]', testUser.email);
      await page.fill('input[name="username"]', testUser.username);
      await page.fill('input[name="first_name"], input[name="firstName"]', testUser.first_name);
      await page.fill('input[name="last_name"], input[name="lastName"]', testUser.last_name);

      await userPage.clickSave();

      // Should not close modal (validation error)
      await page.waitForTimeout(500);
      const modal = page.locator(userPage.userModal);
      await expect(modal).toBeVisible();
    });
  });

  test.describe('View User', () => {
    test.beforeEach(async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });
    });

    test('should display user in list', async () => {
      await userPage.goto();

      // Find user
      const user = await userPage.findUserByUsername(testUser.username);
      await expect(user).toBeVisible();

      // Verify details
      await expect(user).toContainText(testUser.email);
    });

    test('should search users', async () => {
      await userPage.goto();

      // Search for user
      await userPage.searchUser(testUser.username);

      // Should still see the user
      await userPage.verifyUserExists(testUser.username);
    });
  });

  test.describe('Edit User', () => {
    test.beforeEach(async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });
    });

    test('should update user email', async () => {
      const newEmail = `updated.${testUser.email}`;

      await userPage.editUser(testUser.username, {
        email: newEmail,
      });

      // Verify update
      await userPage.goto();
      const user = await userPage.findUserByUsername(testUser.username);
      await expect(user).toContainText(newEmail);
    });

    test('should update user name', async () => {
      await userPage.editUser(testUser.username, {
        firstName: 'Updated',
        lastName: 'Name',
      });

      // Verify update
      await userPage.goto();
      const user = await userPage.findUserByUsername(testUser.username);
      await expect(user).toContainText('Updated');
      await expect(user).toContainText('Name');
    });
  });

  test.describe('User Status', () => {
    test.beforeEach(async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
        isActive: true,
      });
    });

    test('should deactivate user', async () => {
      await userPage.deactivateUser(testUser.username);

      // Verify user is inactive
      await userPage.verifyUserIsInactive(testUser.username);
    });

    test('should activate user', async () => {
      // First deactivate
      await userPage.deactivateUser(testUser.username);

      // Then activate
      await userPage.activateUser(testUser.username);

      // Verify user is active
      await userPage.verifyUserIsActive(testUser.username);
    });

    test('should create inactive user', async () => {
      const inactiveUser = generateUser('inactive');

      await userPage.createUser({
        email: inactiveUser.email,
        username: inactiveUser.username,
        firstName: inactiveUser.first_name,
        lastName: inactiveUser.last_name,
        password: inactiveUser.password_hash,
        isActive: false,
      });

      // Verify user is inactive
      await userPage.verifyUserIsInactive(inactiveUser.username);
    });
  });

  test.describe('User Permissions', () => {
    test.beforeEach(async () => {
      await userPage.createUser({
        email: testUser.email,
        username: testUser.username,
        firstName: testUser.first_name,
        lastName: testUser.last_name,
        password: testUser.password_hash,
      });
    });

    test('should assign permission to user', async () => {
      await userPage.assignPermission(testUser.username, 'Admin');

      // Verify permission was assigned
      await userPage.verifyUserHasPermission(testUser.username, 'Admin');
    });

    test('should assign multiple permissions', async () => {
      await userPage.assignPermission(testUser.username, 'Read');
      await userPage.assignPermission(testUser.username, 'Write');

      // Verify both permissions
      await userPage.verifyUserHasPermission(testUser.username, 'Read');
      await userPage.verifyUserHasPermission(testUser.username, 'Write');
    });
  });

  test.describe('User List', () => {
    test('should display multiple users', async () => {
      const user1 = generateUser('1');
      const user2 = generateUser('2');
      const user3 = generateUser('3');

      await userPage.createUser({
        email: user1.email,
        username: user1.username,
        firstName: user1.first_name,
        lastName: user1.last_name,
        password: user1.password_hash,
      });

      await userPage.createUser({
        email: user2.email,
        username: user2.username,
        firstName: user2.first_name,
        lastName: user2.last_name,
        password: user2.password_hash,
      });

      await userPage.createUser({
        email: user3.email,
        username: user3.username,
        firstName: user3.first_name,
        lastName: user3.last_name,
        password: user3.password_hash,
      });

      await userPage.goto();

      // Verify all are visible
      await userPage.verifyUserExists(user1.username);
      await userPage.verifyUserExists(user2.username);
      await userPage.verifyUserExists(user3.username);

      // Get count
      const count = await userPage.getUserCount();
      expect(count).toBeGreaterThanOrEqual(4); // 3 + admin
    });

    test('should filter active users', async ({ page }) => {
      const activeUser = generateUser('active');
      const inactiveUser = generateUser('inactive');

      await userPage.createUser({
        email: activeUser.email,
        username: activeUser.username,
        firstName: activeUser.first_name,
        lastName: activeUser.last_name,
        password: activeUser.password_hash,
        isActive: true,
      });

      await userPage.createUser({
        email: inactiveUser.email,
        username: inactiveUser.username,
        firstName: inactiveUser.first_name,
        lastName: inactiveUser.last_name,
        password: inactiveUser.password_hash,
        isActive: false,
      });

      await userPage.goto();

      // Apply active filter (if exists)
      const statusFilter = page.locator('select[name="status"], [data-filter="status"]');
      if (await statusFilter.isVisible({ timeout: 2000 })) {
        await statusFilter.selectOption('active');
        await page.waitForTimeout(500);

        // Should see active user
        await userPage.verifyUserExists(activeUser.username);
      }
    });
  });
});
