import { test as base, expect, Page, APIRequestContext } from '@playwright/test';

/**
 * Authentication fixtures for Playwright tests
 * Provides reusable authentication helpers and authenticated contexts
 */

export interface AuthFixtures {
  /**
   * Gets a CSRF token from the API
   */
  getCSRFToken: (request: APIRequestContext) => Promise<string>;

  /**
   * Performs login via UI
   */
  loginViaUI: (page: Page, username: string, password: string) => Promise<void>;

  /**
   * Performs logout via UI
   */
  logoutViaUI: (page: Page) => Promise<void>;

  /**
   * Creates a bearer token for API authentication
   */
  createBearerToken: (request: APIRequestContext, name: string) => Promise<string>;

  /**
   * Makes an authenticated API request with bearer token
   */
  makeAuthRequest: (
    request: APIRequestContext,
    token: string,
    method: string,
    endpoint: string,
    data?: any
  ) => Promise<any>;
}

export const test = base.extend<AuthFixtures>({
  /**
   * Get CSRF token from API
   */
  getCSRFToken: async ({ request }, use) => {
    const getToken = async (requestContext: APIRequestContext): Promise<string> => {
      const baseURL = process.env.BASE_URL || 'http://localhost:8080';
      const response = await requestContext.get(`${baseURL}/api/csrf-token`);
      expect(response.ok()).toBeTruthy();
      const data = await response.json();
      return data.csrf_token;
    };
    await use(getToken);
  },

  /**
   * Login via UI (for testing login functionality)
   */
  loginViaUI: async ({ page }, use) => {
    const login = async (pageContext: Page, username: string, password: string): Promise<void> => {
      const baseURL = process.env.BASE_URL || 'http://localhost:8080';

      // Navigate to app (login dialog should appear)
      await pageContext.goto(baseURL);

      // Wait for login form
      await pageContext.waitForSelector('input[type="text"]', { timeout: 10000 });

      // Fill credentials
      await pageContext.fill('input[type="text"]', username);
      await pageContext.fill('input[type="password"]', password);

      // Submit
      await pageContext.click('button[type="submit"]');

      // Wait for login to complete
      await pageContext.waitForTimeout(2000);

      // Verify login success
      const cookies = await pageContext.context().cookies();
      const hasSession = cookies.some(c => c.name === 'session' || c.name === 'windshift_session');
      expect(hasSession).toBeTruthy();
    };
    await use(login);
  },

  /**
   * Logout via UI
   */
  logoutViaUI: async ({ page }, use) => {
    const logout = async (pageContext: Page): Promise<void> => {
      // Click user menu/avatar
      await pageContext.click('[data-testid="user-menu"], .user-avatar, button:has-text("admin")');

      // Wait for dropdown menu
      await pageContext.waitForSelector('text=Logout, text=Sign out', { timeout: 5000 });

      // Click logout
      await pageContext.click('text=Logout, text=Sign out');

      // Wait for logout to complete
      await pageContext.waitForTimeout(1000);

      // Verify we're logged out (login dialog should appear or redirected)
      await pageContext.waitForSelector('input[type="text"], text=Login', { timeout: 5000 });
    };
    await use(logout);
  },

  /**
   * Create bearer token for API authentication
   */
  createBearerToken: async ({ request }, use) => {
    const createToken = async (
      requestContext: APIRequestContext,
      name: string
    ): Promise<string> => {
      const baseURL = process.env.BASE_URL || 'http://localhost:8080';

      // Get CSRF token
      const csrfResponse = await requestContext.get(`${baseURL}/api/csrf-token`);
      const csrfData = await csrfResponse.json();

      // Create token
      const tokenResponse = await requestContext.post(`${baseURL}/api/api-tokens`, {
        headers: {
          'X-CSRF-Token': csrfData.csrf_token,
        },
        data: {
          name: name,
          permissions: ['read', 'write', 'admin'],
        },
      });

      expect(tokenResponse.ok()).toBeTruthy();
      const tokenData = await tokenResponse.json();
      return tokenData.token;
    };
    await use(createToken);
  },

  /**
   * Make authenticated API request with bearer token
   */
  makeAuthRequest: async ({ request }, use) => {
    const makeRequest = async (
      requestContext: APIRequestContext,
      token: string,
      method: string,
      endpoint: string,
      data?: any
    ): Promise<any> => {
      const baseURL = process.env.BASE_URL || 'http://localhost:8080';
      const url = `${baseURL}/api${endpoint}`;

      const options: any = {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      };

      if (data) {
        options.data = data;
      }

      let response;
      switch (method.toUpperCase()) {
        case 'GET':
          response = await requestContext.get(url, options);
          break;
        case 'POST':
          response = await requestContext.post(url, options);
          break;
        case 'PUT':
          response = await requestContext.put(url, options);
          break;
        case 'DELETE':
          response = await requestContext.delete(url, options);
          break;
        default:
          throw new Error(`Unsupported HTTP method: ${method}`);
      }

      if (!response.ok()) {
        const body = await response.text();
        console.error(`API request failed: ${method} ${endpoint}`, body);
      }

      return response;
    };
    await use(makeRequest);
  },
});

export { expect } from '@playwright/test';
