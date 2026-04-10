/**
 * Public board API - uses plain fetch (no auth headers needed)
 */
export const publicBoard = {
  async get(slug) {
    const res = await fetch(`/api/public/board/${encodeURIComponent(slug)}`);
    if (!res.ok) {
      const err = new Error(`${res.status}`);
      err.status = res.status;
      throw err;
    }
    return res.json();
  },

  async getItem(slug, key) {
    const res = await fetch(
      `/api/public/board/${encodeURIComponent(slug)}/items/${encodeURIComponent(key)}`
    );
    if (!res.ok) {
      const err = new Error(`${res.status}`);
      err.status = res.status;
      throw err;
    }
    return res.json();
  },
};
