import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Mock the api module before importing authStore
vi.mock('../api.js', () => ({
  api: {
    auth: {
      getCurrentUser: vi.fn(),
      login: vi.fn(),
      logout: vi.fn(),
      logoutAll: vi.fn(),
      refreshSession: vi.fn(),
      changePassword: vi.fn(),
    },
  },
}));

import { api } from '../api.js';
// Import after mocking
import { authStore } from './auth.svelte.js';

describe('authStore', () => {
  beforeEach(() => {
    // Reset store state before each test
    authStore.clearAuth();
    vi.clearAllMocks();
  });

  describe('init()', () => {
    it('should fetch user and set authenticated state on success', async () => {
      const mockUser = { id: '1', username: 'testuser', email: 'test@example.com' };
      const mockSession = { id: 'session-1', expires_at: '2025-01-01T00:00:00Z' };

      api.auth.getCurrentUser.mockResolvedValueOnce({
        user: mockUser,
        session: mockSession,
      });

      await authStore.init();

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(true);
      expect(state.user).toEqual(mockUser);
      expect(state.session).toEqual(mockSession);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
      expect(api.auth.getCurrentUser).toHaveBeenCalledTimes(1);
    });

    it('should handle 401/errors gracefully and set unauthenticated state', async () => {
      api.auth.getCurrentUser.mockRejectedValueOnce(new Error('Unauthorized'));

      await authStore.init();

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.user).toBeNull();
      expect(state.session).toBeNull();
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull(); // init() clears error on failure
    });
  });

  describe('login()', () => {
    it('should update user and session on successful login', async () => {
      const mockUser = { id: '1', username: 'testuser' };
      const mockSession = { id: 'session-1' };

      api.auth.login.mockResolvedValueOnce({
        success: true,
        user: mockUser,
      });
      api.auth.getCurrentUser.mockResolvedValueOnce({
        user: mockUser,
        session: mockSession,
      });

      const result = await authStore.login({ username: 'testuser', password: 'password123' });

      expect(result.success).toBe(true);
      const state = get(authStore);
      expect(state.isAuthenticated).toBe(true);
      expect(state.user).toEqual(mockUser);
      expect(state.session).toEqual(mockSession);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should handle invalid credentials', async () => {
      api.auth.login.mockResolvedValueOnce({
        success: false,
        message: 'Invalid username or password',
      });

      const result = await authStore.login({ username: 'wrong', password: 'wrong' });

      expect(result.success).toBe(false);
      expect(result.message).toBe('Invalid username or password');
      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBe('Invalid username or password');
      expect(state.loading).toBe(false);
    });

    it('should handle login API errors', async () => {
      api.auth.login.mockRejectedValueOnce(new Error('Network error'));

      const result = await authStore.login({ username: 'user', password: 'pass' });

      expect(result.success).toBe(false);
      expect(result.message).toBe('Network error');
      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBe('Network error');
    });
  });

  describe('logout()', () => {
    it('should clear state on successful logout', async () => {
      // First set up authenticated state
      const mockUser = { id: '1', username: 'testuser' };
      authStore.setAuthData(mockUser, { id: 'session-1' });

      api.auth.logout.mockResolvedValueOnce({});

      await authStore.logout();

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.user).toBeNull();
      expect(state.session).toBeNull();
      expect(state.loading).toBe(false);
      expect(api.auth.logout).toHaveBeenCalledTimes(1);
    });

    it('should clear state even if API call fails', async () => {
      // First set up authenticated state
      const mockUser = { id: '1', username: 'testuser' };
      authStore.setAuthData(mockUser, { id: 'session-1' });

      // Mock console.warn to verify it's called
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
      api.auth.logout.mockRejectedValueOnce(new Error('Network error'));

      await authStore.logout();

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.user).toBeNull();
      expect(state.session).toBeNull();
      expect(warnSpy).toHaveBeenCalledWith('Logout API call failed:', expect.any(Error));

      warnSpy.mockRestore();
    });
  });

  describe('clearAuth()', () => {
    it('should reset all stores and set session expired error', () => {
      // First set up authenticated state
      const mockUser = { id: '1', username: 'testuser' };
      authStore.setAuthData(mockUser, { id: 'session-1' });

      // Verify we're authenticated
      expect(get(authStore).isAuthenticated).toBe(true);

      // Clear auth (simulating session expiry)
      authStore.clearAuth();

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(false);
      expect(state.user).toBeNull();
      expect(state.session).toBeNull();
      expect(state.loading).toBe(false);
      expect(state.error).toBe('Session expired. Please log in again.');
    });
  });

  describe('setAuthData()', () => {
    it('should set user and session directly (for FIDO login)', () => {
      const mockUser = { id: '1', username: 'fidouser' };
      const mockSession = { id: 'fido-session-1' };

      authStore.setAuthData(mockUser, mockSession);

      const state = get(authStore);
      expect(state.isAuthenticated).toBe(true);
      expect(state.user).toEqual(mockUser);
      expect(state.session).toEqual(mockSession);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
    });
  });

  describe('refreshSession()', () => {
    it('should update session on successful refresh', async () => {
      const mockSession = { id: 'refreshed-session', expires_at: '2025-02-01T00:00:00Z' };

      api.auth.refreshSession.mockResolvedValueOnce({});
      api.auth.getCurrentUser.mockResolvedValueOnce({
        user: { id: '1' },
        session: mockSession,
      });

      const result = await authStore.refreshSession(true);

      expect(result).toBe(true);
      expect(api.auth.refreshSession).toHaveBeenCalledWith({ remember_me: true });
      const state = get(authStore);
      expect(state.session).toEqual(mockSession);
    });

    it('should return false on refresh failure', async () => {
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
      api.auth.refreshSession.mockRejectedValueOnce(new Error('Session invalid'));

      const result = await authStore.refreshSession();

      expect(result).toBe(false);
      warnSpy.mockRestore();
    });
  });

  describe('changePassword()', () => {
    it('should return success on password change', async () => {
      api.auth.changePassword.mockResolvedValueOnce({
        message: 'Password updated successfully',
      });

      const result = await authStore.changePassword({
        current_password: 'oldpass',
        new_password: 'newpass',
      });

      expect(result.success).toBe(true);
      expect(result.message).toBe('Password updated successfully');
      const state = get(authStore);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should return failure on password change error', async () => {
      api.auth.changePassword.mockRejectedValueOnce(new Error('Current password is incorrect'));

      const result = await authStore.changePassword({
        current_password: 'wrongpass',
        new_password: 'newpass',
      });

      expect(result.success).toBe(false);
      expect(result.message).toBe('Current password is incorrect');
      const state = get(authStore);
      expect(state.error).toBe('Current password is incorrect');
    });
  });

  describe('convenience getters', () => {
    it('should provide currentUser getter', () => {
      const mockUser = { id: '1', username: 'testuser' };
      authStore.setAuthData(mockUser, { id: 'session-1' });

      expect(authStore.currentUser).toEqual(mockUser);
    });

    it('should provide isAuthenticated getter', () => {
      expect(authStore.isAuthenticated).toBe(false);

      authStore.setAuthData({ id: '1' }, { id: 'session-1' });

      expect(authStore.isAuthenticated).toBe(true);
    });
  });
});
