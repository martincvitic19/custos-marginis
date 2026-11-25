// One function for every API call: JSON in/out, the CSRF token on every
// state-changing request, errors turned into exceptions with the server's
// message. The CSRF token lives only in memory (sessionStorage as a
// fallback across reloads of the same tab).
"use strict";
const Api = (() => {
  let csrf = null;
  try { csrf = sessionStorage.getItem("csrf"); } catch (e) { /* storage blocked */ }
  function setCsrf(t) { csrf = t; try { t ? sessionStorage.setItem("csrf", t) : sessionStorage.removeItem("csrf"); } catch (e) { /* ignore */ } }
  async function call(method, path, body) {
    const opt = { method, headers: {}, credentials: "same-origin" };
    if (body !== undefined) { opt.headers["Content-Type"] = "application/json"; opt.body = JSON.stringify(body); }
    if (method !== "GET" && csrf) opt.headers["X-CSRF-Token"] = csrf;
    const r = await fetch(path, opt);
    const ct = r.headers.get("Content-Type") || "";
    const data = ct.includes("json") ? await r.json() : await r.text();
    if (!r.ok) {
      const e = new Error((data && data.error) || r.statusText);
      e.status = r.status; throw e;
    }
    return data;
  }
  return {
    get: (p) => call("GET", p),
    post: (p, b) => call("POST", p, b === undefined ? {} : b),
    put: (p, b) => call("PUT", p, b),
    del: (p) => call("DELETE", p),
    setCsrf,
  };
})();
