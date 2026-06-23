// Cloudflare Worker for short-link.fun (FE).
//
// Two jobs:
//   1. GET /{code}  -> resolve the short code against the BE and 302-redirect
//      to the original URL, so links read as https://short-link.fun/GeAi9K.
//   2. everything else -> serve the static landing page from ./web (ASSETS).
//
// The BE (api.short-link.fun) owns encode/decode; this Worker only fronts the
// apex so the short links can live on the bare domain.

const API = "https://api.short-link.fun";
// 6-char base62 — must match the BE's code format exactly.
const CODE = /^\/[0-9A-Za-z]{6}$/;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (request.method === "GET" && CODE.test(url.pathname)) {
      // Ask the BE to resolve the code, then redirect ourselves.
      const res = await fetch(`${API}/decode`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ short_url: `${API}${url.pathname}` }),
      });

      if (res.ok) {
        const { original_url } = await res.json();
        return Response.redirect(original_url, 302);
      }
      // Unknown/invalid code: fall through to the static site so the user
      // lands on the homepage rather than a raw JSON error.
    }

    // Landing page and any other static asset.
    return env.ASSETS.fetch(request);
  },
};
