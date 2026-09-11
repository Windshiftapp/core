import { afterEach, describe, expect, it, vi } from 'vitest';
import { zammadConnections, zammadTickets } from './integrations.js';

describe('zammadConnections.startOAuth', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts the system connection OAuth flow with an empty POST body', async () => {
    const authURL = 'https://zammad.example.test/oauth/authorize?state=state-value';
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ auth_url: authURL }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadConnections.startOAuth('connection-id')).resolves.toEqual({
      auth_url: authURL,
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/admin/integration-providers/connection-id/oauth/start');
    expect(options.method).toBe('POST');
    expect(options.body).toBeUndefined();
  });
});

describe('zammadConnections.refreshAllTickets', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts a system-wide ticket refresh', async () => {
    const result = { started: true };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(result), {
        status: 202,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadConnections.refreshAllTickets()).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/admin/zammad-ticket-links/refresh');
    expect(options.method).toBe('POST');
    expect(options.body).toBeUndefined();
  });
});

describe('Zammad ticket-link API', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('resolves a Zammad correlation key to its current item', async () => {
    const destination = { workspace_id: 7, item_id: 42 };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(destination), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadTickets.resolve('windshift:provider:TST-42')).resolves.toEqual(destination);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/zammad-ticket-links/resolve/windshift%3Aprovider%3ATST-42'
    );
  });

  it('loads assignable owners for the selected group', async () => {
    const owners = [
      { id: 1, name: 'Not assigned' },
      { id: 7, name: 'Ada Lovelace' },
    ];
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(owners), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadConnections.owners(23, 'connection-id', 42)).resolves.toEqual(owners);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/workspaces/23/zammad-connections/connection-id/owners?group_id=42'
    );
  });

  it('links an existing ticket by number', async () => {
    const response = { id: 'link-id', ticket_number: '12345', sync_state: 'linked' };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(
      zammadTickets.link(99, { connection_id: 'connection-id', ticket_number: '12345' })
    ).resolves.toEqual(response);

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/items/99/zammad-ticket-links');
    expect(options).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ connection_id: 'connection-id', ticket_number: '12345' }),
    });
  });

  it('updates only the selected ticket fields', async () => {
    const response = { id: 'link-id', group_id: 42, owner_id: 7 };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadTickets.update('link-id', { group_id: 42, owner_id: 7 })).resolves.toEqual(
      response
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/zammad-ticket-links/link-id');
    expect(options).toMatchObject({
      method: 'PUT',
      body: JSON.stringify({ group_id: 42, owner_id: 7 }),
    });
  });

  it('removes only the Zammad ticket assignment', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(zammadTickets.delete('link-id')).resolves.toBeNull();

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/zammad-ticket-links/link-id');
    expect(options.method).toBe('DELETE');
  });
});
