<script>
  import { customerAccess } from '../api/customerAccess.js';
  import { isWebAuthnSupported, prepareCredentialRequestOptions, processCredentialRequestResponse } from '../utils/webauthn-utils.js';
  import CustomerGitHubButtons from './CustomerGitHubButtons.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  let { kind, slug, status, onlogin } = $props();
  let email = $state('');
  let busy = $state(false);
  let message = $state('');
  let error = $state(new URLSearchParams(window.location.search).get('customer_login_error') || '');

  async function magic(event) {
    event.preventDefault();
    busy = true;
    error = '';
    try { message = (await customerAccess.login(kind, slug, 'request', { email })).message; }
    catch (err) { error = err.message; }
    finally { busy = false; }
  }
  async function passkey() {
    busy = true;
    error = '';
    try {
      const challenge = await customerAccess.login(kind, slug, 'passkey/start');
      const credential = await navigator.credentials.get(prepareCredentialRequestOptions(challenge.publicKey));
      if (!(credential instanceof PublicKeyCredential)) throw new Error('Sign-in was cancelled.');
      await customerAccess.login(kind, slug, 'passkey/complete', {
        sessionId: challenge.sessionId, response: processCredentialRequestResponse(credential),
      });
      await onlogin();
    } catch (err) { error = err.message; }
    finally { busy = false; }
  }
</script>

<div class="space-y-4" data-testid="customer-login">
  <h1 class="text-xl font-semibold">Customer sign-in required</h1>
  <p class="text-sm text-ds-text-subtle">Sign in with a customer account from an approved organisation. Contact your administrator if you need access.</p>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  {#if message}<p role="status" data-testid="customer-login-message">{message}</p>{/if}
  <CustomerGitHubButtons {kind} {slug} providers={status.providers} />
  <form onsubmit={magic} class="space-y-3">
    <label for="customer-login-email">Email</label>
    <Input id="customer-login-email" type="email" required bind:value={email} disabled={busy} />
    <Button type="submit" variant="primary" disabled={busy} dataTestid="customer-login-magic">Email me a sign-in link</Button>
  </form>
  {#if status.passkeys && isWebAuthnSupported()}
    <Button variant="secondary" disabled={busy} onclick={passkey} dataTestid="customer-login-passkey">Sign in with a customer passkey</Button>
  {/if}
</div>
