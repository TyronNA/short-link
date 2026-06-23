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

    // Browsers navigate with GET; link-preview bots and prefetchers use HEAD.
    // Both must resolve, or HEAD requests wrongly hit the 404 page.
    if ((request.method === "GET" || request.method === "HEAD") && CODE.test(url.pathname)) {
      // A code-shaped path is a link resolution — its outcome must NEVER be
      // cached at the edge. The 404.html that ASSETS serves carries
      // `cache-control: public, must-revalidate`, so a single fall-through
      // here used to get cached by Cloudflare against the path, and every
      // later visit (browser navigations send `Accept: text/html`, which keys
      // a different cache entry than a bare curl) served that stale 404 —
      // surviving even an incognito window, since edge cache ignores it.
      // Resolving the code in-Worker and stamping `no-store` on every branch
      // guarantees the response is always live.
      const noStore = (init) =>
        new Response(init.body ?? null, {
          status: init.status,
          headers: { ...(init.headers || {}), "Cache-Control": "no-store" },
        });

      // Ask the BE to resolve the code, then redirect ourselves.
      const res = await fetch(`${API}/decode`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ short_url: `${API}${url.pathname}` }),
      });

      if (res.ok) {
        const { original_url } = await res.json();
        return noStore({ status: 302, headers: { Location: original_url } });
      }

      // Valid 6-char code the BE doesn't know (404), or any BE error (5xx):
      // serve the 404 page, but never let it be cached against this path.
      const notFound = await env.ASSETS.fetch(new URL("/404.html", url));
      return noStore({
        status: 404,
        body: notFound.body,
        headers: { "Content-Type": "text/html; charset=utf-8" },
      });
    }

    // Landing page and any other static asset.
    return env.ASSETS.fetch(request);
  },
};
