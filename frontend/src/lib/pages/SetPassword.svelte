<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import TextField from '../components/TextField.svelte';
  import Logo from '../components/Logo.svelte';
  import { Lock, CheckCircle, AlertCircle, Loader2 } from '@lucide/svelte';

  let token = $derived($currentRoute.params.token);
  let loading = $state(true);
  let verifying = $state(true);
  let success = $state(false);
  let user = $state(null);
  let error = $state(null);

  let password = $state('');
  let confirmPassword = $state('');
  let isSubmitting = $state(false);

  const isPasswordValid = $derived(password.length >= 8);
  const passwordsMatch = $derived(password === confirmPassword);
  const canSubmit = $derived(isPasswordValid && passwordsMatch && !isSubmitting);

  onMount(async () => {
    try {
      // Verify token
      const response = await fetch(`/api/auth/invitations/verify?token=${token}`);
      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.message || 'Invalid or expired invitation link');
      }
      user = await response.json();
      verifying = false;
    } catch (err) {
      error = err.message;
      verifying = false;
    } finally {
      loading = false;
    }
  });

  async function handleSubmit(e) {
    if (e) e.preventDefault();
    if (!canSubmit) return;

    isSubmitting = true;
    try {
      const response = await fetch('/api/auth/invitations/accept', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          token,
          password
        })
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.message || 'Failed to set password');
      }

      success = true;
      successToast('Password set successfully! You can now log in.');
      
      // Redirect to login after a short delay
      setTimeout(() => {
        navigate('/');
      }, 3000);
    } catch (err) {
      errorToast(err.message);
    } finally {
      isSubmitting = false;
    }
  }
</script>

<div class="min-h-screen flex flex-col items-center justify-center p-4 bg-[var(--ds-surface)]">
  <div class="w-full max-w-md bg-[var(--ds-surface-raised)] rounded-2xl shadow-xl overflow-hidden">
    <div class="p-8 flex flex-col items-center">
      <div class="mb-8">
        <Logo size="large" />
      </div>

      {#if verifying}
        <div class="flex flex-col items-center py-12">
          <Loader2 class="w-12 h-12 text-[var(--ds-icon)] animate-spin mb-4" />
          <p class="text-[var(--ds-text-subtle)]">Verifying invitation...</p>
        </div>
      {:else if error}
        <div class="flex flex-col items-center py-12 text-center">
          <div class="w-16 h-16 bg-[var(--ds-danger-subtle)] rounded-full flex items-center justify-center mb-6 text-[var(--ds-text-danger)]">
            <AlertCircle class="w-10 h-10" />
          </div>
          <h1 class="text-2xl font-bold text-[var(--ds-text)] mb-2">Invalid Invitation</h1>
          <p class="text-[var(--ds-text-subtle)] mb-8">{error}</p>
          <Button variant="primary" onclick={() => navigate('/')}>
            Back to Login
          </Button>
        </div>
      {:else if success}
        <div class="flex flex-col items-center py-12 text-center">
          <div class="w-16 h-16 bg-[var(--ds-success-subtle)] rounded-full flex items-center justify-center mb-6 text-[var(--ds-text-success)]">
            <CheckCircle class="w-10 h-10" />
          </div>
          <h1 class="text-2xl font-bold text-[var(--ds-text)] mb-2">Password Set!</h1>
          <p class="text-[var(--ds-text-subtle)] mb-8">Your account has been activated. Redirecting you to the login page...</p>
          <Button variant="primary" onclick={() => navigate('/')}>
            Login Now
          </Button>
        </div>
      {:else}
        <div class="w-full">
          <h1 class="text-2xl font-bold text-[var(--ds-text)] mb-2 text-center">Welcome, {user.first_name}!</h1>
          <p class="text-[var(--ds-text-subtle)] mb-8 text-center">Please set a password for your account to get started.</p>

          <form onsubmit={handleSubmit} class="space-y-6">
            <TextField
              label="Email Address"
              value={user.email}
              disabled
            />

            <TextField
              label="New Password"
              type="password"
              bind:value={password}
              placeholder="Min. 8 characters"
              required
              autofocus
            />

            <TextField
              label="Confirm Password"
              type="password"
              bind:value={confirmPassword}
              placeholder="Repeat password"
              required
            />
            {#if confirmPassword && !passwordsMatch}
              <p class="text-xs text-[var(--ds-text-danger)] mt-1">Passwords don't match</p>
            {/if}

            {#if password && !isPasswordValid}
              <p class="text-xs text-[var(--ds-text-danger)] mt-1">Password must be at least 8 characters long.</p>
            {/if}

            <Button
              type="submit"
              variant="primary"
              class="w-full"
              disabled={!canSubmit}
              loading={isSubmitting}
            >
              Activate Account
            </Button>
          </form>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  :global(body) {
    margin: 0;
    padding: 0;
  }
</style>
