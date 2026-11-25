// Custos UI. Plain JavaScript, no framework. Every value from the API is
// put into the page with textContent (via h()), never innerHTML, so a
// device name cannot inject markup.
"use strict";

const ROLES = { child: "dijete", guest: "gost", trusted: "odrasla osoba", unfiltered: "bez filtra" };
const DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];
const DAY_HR = { sun: "ned", mon: "pon", tue: "uto", wed: "sri", thu: "čet", fri: "pet", sat: "sub" };

function h(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else if (k === "class") e.className = v;
    else if (v === true) e.setAttribute(k, "");
    else if (v !== false && v != null) e.setAttribute(k, v);
  }
  for (const k of kids.flat()) if (k != null) e.append(k instanceof Node ? k : String(k));
  return e;
}
const main = () => document.getElementById("main");
function show(...nodes) { main().replaceChildren(...nodes); }
function toast(msg) {
  const t = document.getElementById("toast");
  t.textContent = msg; t.hidden = false;
  clearTimeout(toast.t); toast.t = setTimeout(() => { t.hidden = true; }, 4000);
}
function card(...k) { return h("div", { class: "card" }, ...k); }
function roleTag(r) { return h("span", { class: "tag " + r }, ROLES[r] || r); }
function fmtTime(s) { try { return new Date(s).toLocaleString("hr-HR"); } catch (e) { return s; } }
async function guarded(fn) { try { await fn(); } catch (e) { if (e.status === 401) return route("login"); toast("Greška: " + e.message); } }

// ---- login / first run -----------------------------------------------------

async function pageLogin() {
  const st = await Api.get("/api/setup");
  document.getElementById("nav").hidden = true;
  const pw = h("input", { type: "password", autocomplete: st.password_set ? "current-password" : "new-password", required: true, minlength: st.password_set ? 1 : 10 });
  const pw2 = h("input", { type: "password", autocomplete: "new-password", minlength: 10 });
  const form = h("form", { class: "row", onsubmit: (ev) => { ev.preventDefault(); guarded(async () => {
    let r;
    if (!st.password_set) {
      if (pw.value !== pw2.value) throw new Error("lozinke se ne podudaraju");
      r = await Api.post("/api/setup", { password: pw.value });
    } else {
      r = await Api.post("/api/login", { password: pw.value });
    }
    Api.setCsrf(r.csrf);
    location.hash = st.wizard_done ? "#pregled" : "#wizard";
    route();
  }); } },
    h("label", {}, "Lozinka ", pw),
    st.password_set ? null : h("label", {}, "Ponovi ", pw2),
    h("button", { type: "submit" }, st.password_set ? "Prijava" : "Postavi lozinku"));
  show(h("h1", {}, st.password_set ? "Prijava" : "Dobro došli u Custos"),
    card(st.password_set ? h("p", {}, "Upravljanje je dostupno samo iz mreže odraslih (VLAN 20) i iz VPN-a.")
      : h("p", {}, "Custos nema tvorničku lozinku. Postavite vlastitu (najmanje 10 znakova). Nakon toga slijedi kratki vodič za operaterov router."),
    form));
}

// ---- overview ---------------------------------------------------------------

async function pagePregled() {
  const s = await Api.get("/api/status");
  const a = s.apply || {};
  const alerts = (s.alerts_24h || []).map((e) => h("tr", {},
    h("td", {}, fmtTime(e.time)), h("td", { class: e.severity === "alert" ? "bad" : "warn" }, e.kind), h("td", {}, e.device || e.mac || e.ip || ""), h("td", {}, e.message)));
  show(h("h1", {}, "Pregled"),
    h("div", { class: "grid" },
      card(h("div", { class: "stat" }, h("span", { class: "muted" }, "Zaštita (DNS filter)"),
        h("b", { class: s.adguardhome.running && s.adguardhome.protection_enabled ? "ok" : "bad" }, s.adguardhome.running ? "radi" : "NE RADI"))),
      card(h("div", { class: "stat" }, h("span", { class: "muted" }, "Zadnja primjena pravila"),
        h("b", { class: a.ok ? "ok" : "bad" }, a.ok ? "u redu" : "greška"), h("span", { class: "muted" }, a.time ? fmtTime(a.time) : ""))),
      card(h("div", { class: "stat" }, h("span", { class: "muted" }, "Upozorenja (24 h)"),
        h("b", { class: alerts.length ? "warn" : "ok" }, String(alerts.length)))),
      card(h("div", { class: "stat" }, h("span", { class: "muted" }, "Internet (wan)"), h("b", {}, s.wan_address || "—"))),
      card(h("div", { class: "stat" }, h("span", { class: "muted" }, "Odbačeno na ulazu (XDP)"),
        s.xdp ? h("b", { class: "ok" }, String((s.xdp.bogon || 0) + (s.xdp.tif || 0) + (s.xdp.syn || 0))) : h("b", { class: "muted" }, "—"),
        s.xdp ? h("span", { class: "muted" }, "lažne adrese " + (s.xdp.bogon || 0) + ", poznati napadači " + (s.xdp.tif || 0) + ", SYN flood " + (s.xdp.syn || 0) + " · " + (s.xdp.mode || "")) : null))),
    a.ok ? null : card(h("p", { class: "bad" }, "Greška pri primjeni: ", a.error || "")),
    h("h2", {}, "Upozorenja"),
    card(alerts.length ? h("div", { class: "scroll" }, h("table", {}, h("tr", {}, h("th", {}, "Vrijeme"), h("th", {}, "Vrsta"), h("th", {}, "Uređaj"), h("th", {}, "Opis")), alerts))
      : h("p", { class: "muted" }, "Nema upozorenja u zadnja 24 sata.")),
    h("p", { class: "muted" }, "Dnevnik događaja za prijavu incidenta (GDPR čl. 33): ", h("a", { href: "/api/events.csv" }, "preuzmi CSV")));
}

// ---- devices ----------------------------------------------------------------

function scheduleText(list) {
  if (!list || !list.length) return "bez ograničenja";
  return list.map((s) => s.days.map((d) => DAY_HR[d]).join(",") + " " + s.from + "–" + s.to).join("; ");
}

async function pageUredaji() {
  const d = await Api.get("/api/devices");
  const rows = d.devices.map((x) => {
    const roleSel = h("select", {}, Object.keys(ROLES).map((r) => h("option", { value: r, selected: r === x.role }, ROLES[r])));
    const name = h("input", { value: x.name || "", placeholder: "ime (npr. Ivin tablet)", maxlength: 40, size: 16 });
    const save = h("button", { class: "sec", type: "button", onclick: () => guarded(async () => {
      await Api.post("/api/devices", { name: name.value.trim(), mac: x.mac, vlan: x.vlan, role: roleSel.value });
      toast("Spremljeno."); pageUredaji();
    }) }, x.known ? "Spremi" : "Dodaj");
    const extra = h("button", { class: "sec", type: "button", onclick: () => guarded(async () => {
      await Api.post("/api/curfew/override", { device: x.name, minutes: 30 }); toast("Još 30 minuta za " + x.name + "."); pageUredaji();
    }) }, "+30 min");
    return h("tr", {},
      h("td", {}, name, x.known ? null : h("div", { class: "muted" }, "nepoznat → pravila za dijete")),
      h("td", {}, h("div", {}, x.ip || "—"), h("div", { class: "muted" }, x.mac + " · VLAN " + x.vlan)),
      h("td", {}, roleSel),
      h("td", { class: x.curfew_now ? "warn" : "" }, x.curfew_now ? "zabrana sada" : "dopušteno",
        x.curfew_override_until ? h("div", { class: "muted" }, "produljeno do " + fmtTime(x.curfew_override_until)) : null),
      h("td", {}, save, " ", x.known && x.curfew_now ? extra : null,
        x.known ? h("button", { class: "link", type: "button", onclick: () => guarded(async () => {
          if (!confirm("Ukloniti " + x.name + "? Uređaj postaje nepoznat (pravila za dijete).")) return;
          await Api.del("/api/devices/" + encodeURIComponent(x.name)); pageUredaji();
        }) }, " ukloni") : null));
  });
  const roles = await Api.get("/api/roles");
  const roleCards = ["child", "guest"].map((r) => {
    const R = roles[r];
    const strict = h("input", { type: "checkbox", checked: R.strict_egress });
    const sched = h("textarea", { rows: 3 }, JSON.stringify(R.schedules, null, 1));
    return card(h("h2", {}, "Uloga: ", ROLES[r]),
      h("p", { class: "muted" }, "Zabrana interneta: ", scheduleText(R.schedules)),
      h("label", { class: "row" }, strict, " samo web (80/443) i NTP; ostalo, uključujući većinu VPN-ova, blokirano"),
      h("details", {}, h("summary", {}, "Uredi raspored (JSON: dani sun..sat, od, do)"), sched),
      h("button", { type: "button", onclick: () => guarded(async () => {
        await Api.put("/api/roles/" + r, Object.assign({}, R, { strict_egress: strict.checked, schedules: JSON.parse(sched.value) }));
        toast("Uloga spremljena."); pageUredaji();
      }) }, "Spremi ulogu"));
  });
  show(h("h1", {}, "Uređaji"),
    card(h("p", { class: "muted" }, "Uređaj se prepoznaje po MAC adresi i mreži (VLAN). Svaki nepoznati uređaj dobiva pravila za dijete. Ulogu „odrasla osoba” može imati samo uređaj u mreži odraslih (VLAN " + d.admin_vlan + ")."),
      h("div", { class: "scroll" }, h("table", {}, h("tr", {}, h("th", {}, "Uređaj"), h("th", {}, "Adresa"), h("th", {}, "Uloga"), h("th", {}, "Curfew"), h("th", {}, "")), rows))),
    ...roleCards);
}

// ---- filtering --------------------------------------------------------------

async function pageFiltriranje() {
  const f = await Api.get("/api/filtering");
  const lists = f.lists.map((l) => { const cb = h("input", { type: "checkbox", checked: l.enabled }); cb.dataset.url = l.url; return { l, cb }; });
  const ss = h("input", { type: "checkbox", checked: f.safesearch });
  const rules = h("textarea", { rows: 5, placeholder: "||primjer.com^   (blokiraj)\n@@||skola.hr^   (dopusti)" }, f.rules.join("\n"));
  const domain = h("input", { placeholder: "npr. tiktok.com" });
  const out = h("pre", { hidden: true });
  show(h("h1", {}, "Filtriranje"),
    card(h("h2", {}, "Liste"), lists.map(({ l, cb }) => h("label", { class: "row" }, cb, " ", l.name)),
      h("label", { class: "row" }, ss, " SafeSearch (Google, YouTube, Bing, …) za djecu i nepoznate uređaje"),
      h("h2", {}, "Vlastita pravila"), rules,
      h("div", { class: "row" }, h("button", { type: "button", onclick: () => guarded(async () => {
        const body = Object.assign({}, f, { safesearch: ss.checked, rules: rules.value.split("\n").map((s) => s.trim()).filter(Boolean),
          lists: lists.map(({ l, cb }) => Object.assign({}, l, { enabled: cb.checked })) });
        await Api.put("/api/filtering", body); toast("Spremljeno.");
      }) }, "Spremi"))),
    card(h("h2", {}, "Provjeri domenu"),
      h("form", { class: "row", onsubmit: (ev) => { ev.preventDefault(); guarded(async () => {
        const r = await Api.get("/api/filtering/check?domain=" + encodeURIComponent(domain.value.trim()));
        out.hidden = false; out.textContent = (r.reason || "") + (r.rules && r.rules.length ? "\n" + r.rules.map((x) => x.text).join("\n") : "");
      }); } }, domain, h("button", { type: "submit" }, "Provjeri")), out));
}

// ---- activity -----------------------------------------------------------------

async function pageAktivnost() {
  const q = await Api.get("/api/querylog?hours=24");
  show(h("h1", {}, "Aktivnost (24 h)"),
    card(h("p", { class: "muted" }, "Sažetak po uređaju: broj upita i što je blokirano. Custos namjerno ne prikazuje cijelu povijest pregledavanja (GDPR: minimizacija podataka); zapisi se brišu nakon 7 dana."),
      h("div", { class: "scroll" }, h("table", {}, h("tr", {}, h("th", {}, "Uređaj"), h("th", {}, "Upita"), h("th", {}, "Blokirano"), h("th", {}, "Najčešće blokirano")),
        q.devices.map((d) => h("tr", {}, h("td", {}, d.name || d.client, d.name ? h("div", { class: "muted" }, d.client) : null),
          h("td", {}, String(d.total)), h("td", { class: d.blocked ? "warn" : "" }, String(d.blocked)),
          h("td", {}, d.top_blocked.map((t) => h("div", {}, t.domain + " (" + t.count + ")")))))))));
}

// ---- settings: VPN, Wi-Fi, password ---------------------------------------

async function pagePostavke() {
  const v = await Api.get("/api/vpn");
  const wf = await Api.get("/api/wifi");
  const conn = await Api.get("/api/connection");
  const endpoint = h("input", { value: v.vpn.endpoint, placeholder: "javna IP adresa ili ime", size: 24 });
  const exit = h("select", {}, h("option", { value: "mullvad", selected: v.vpn.exit === "mullvad" }, "Mullvad (kill switch)"), h("option", { value: "wan", selected: v.vpn.exit === "wan" }, "kućna linija"));
  const pname = h("input", { placeholder: "ime (npr. Ivin mobitel)", maxlength: 40 });
  const prole = h("select", {}, Object.keys(ROLES).map((r) => h("option", { value: r }, ROLES[r])));
  const peerOut = h("div");
  const cg = h("pre", { hidden: true });
  const oldPw = h("input", { type: "password", autocomplete: "current-password" });
  const newPw = h("input", { type: "password", autocomplete: "new-password", minlength: 10 });
  show(h("h1", {}, "Postavke"),
    card(h("h2", {}, "VPN (izvan kuće)"),
      h("p", { class: "muted" }, "Mobitel se spaja kući u Custos i dobiva ista pravila kao doma. Mullvad tunel: ", conn.mullvad_up ? h("span", { class: "ok" }, "spojen") : h("span", { class: "warn" }, "nije konfiguriran")),
      h("div", { class: "row" }, h("label", {}, "Adresa ", endpoint), h("label", {}, "Izlaz ", exit),
        h("button", { type: "button", onclick: () => guarded(async () => {
          await Api.put("/api/vpn", { enabled: true, endpoint: endpoint.value.trim(), exit: exit.value, port: v.vpn.port }); toast("Spremljeno.");
        }) }, "Spremi"),
        h("button", { class: "sec", type: "button", onclick: () => guarded(async () => {
          cg.hidden = false; cg.textContent = "Provjera…"; const r = await Api.get("/api/connection?cgnat=1"); cg.textContent = JSON.stringify(r.cgnat, null, 2);
        }) }, "Provjeri CGNAT")), cg,
      h("table", {}, h("tr", {}, h("th", {}, "Uređaj"), h("th", {}, "Adresa"), h("th", {}, "Uloga"), h("th", {}, "")),
        v.vpn.peers.map((p) => h("tr", {}, h("td", {}, p.name), h("td", {}, p.address), h("td", {}, roleTag(p.role)),
          h("td", {}, h("button", { class: "link", type: "button", onclick: () => guarded(async () => {
            if (!confirm("Ukloniti " + p.name + "?")) return; await Api.del("/api/vpn/peers/" + encodeURIComponent(p.name)); pagePostavke();
          }) }, "ukloni"))))),
      h("form", { class: "row", onsubmit: (ev) => { ev.preventDefault(); guarded(async () => {
        const r = await Api.post("/api/vpn/peers", { name: pname.value.trim(), role: prole.value });
        peerOut.replaceChildren(h("p", {}, "Skenirajte u WireGuard aplikaciji. Privatni ključ se ne sprema na Custos: ovo je jedina prilika."),
          h("img", { class: "qr", src: r.qr_png, alt: "QR kod WireGuard konfiguracije" }), h("pre", {}, r.config));
      }); } }, pname, prole, h("button", { type: "submit" }, "Novi VPN uređaj")), peerOut),
    card(h("h2", {}, "Wi-Fi"),
      wf.radio ? null : h("p", { class: "warn" }, "Wi-Fi modul nije pronađen."),
      h("table", {}, h("tr", {}, h("th", {}, "Mreža"), h("th", {}, "VLAN"), h("th", {}, "Lozinka")),
        wf.ssids.map((s) => h("tr", {}, h("td", {}, s.name), h("td", {}, String(s.vlan)), h("td", {}, h("code", {}, s.passphrase || "—"))))),
      h("p", { class: "muted" }, "WPA3/WPA2 sa zaštitom upravljačkih okvira. Lozinke su nasumične, generirane na uređaju.")),
    card(h("h2", {}, "Lozinka"),
      h("form", { class: "row", onsubmit: (ev) => { ev.preventDefault(); guarded(async () => {
        await Api.post("/api/password", { old: oldPw.value, new: newPw.value }); Api.setCsrf(null); toast("Lozinka promijenjena, prijavite se ponovno."); route("login");
      }); } }, h("label", {}, "Stara ", oldPw), h("label", {}, "Nova ", newPw), h("button", { type: "submit" }, "Promijeni"))));
}

// ---- first-run wizard: ISP checklist ---------------------------------------

const ISP_STEPS = [
  ["isp_wifi_off", "Ugasite Wi-Fi na operaterovom routeru", "Inače se djeca mogu spojiti na njega i zaobići Custos. Custos to provjerava skeniranjem (kad ima Wi-Fi modul)."],
  ["isp_admin_password", "Promijenite admin lozinku operaterovog routera", "Tvornička lozinka je često na naljepnici. Ovo Custos ne može provjeriti."],
  ["only_custos_on_isp_lan", "Na operaterov router spojite samo Custos", "Svaki drugi uređaj na njemu prolazi mimo zaštite. Custos skenira tu mrežu i javlja strane uređaje."],
  ["bridge_or_dmz", "Bridge način rada ili DMZ prema Custosu (192.168.0.15)", "Bridge je najbolji; ako ga operater ne nudi, DMZ."],
  ["vpn_port_forward", "Proslijedite UDP port VPN-a (51820) na 192.168.0.15", "Potrebno samo za VPN izvan kuće. Provjerite CGNAT u Postavkama."],
];

async function pageWizard() {
  const w = await Api.get("/api/wizard");
  const boxes = {};
  const items = ISP_STEPS.map(([id, title, why]) => {
    const auto = w.auto[id];
    boxes[id] = h("input", { type: "checkbox", checked: !!w.steps[id] });
    return h("div", { class: "check" }, boxes[id], h("div", {}, h("b", {}, title), h("div", { class: "muted" }, why),
      auto ? h("div", { class: auto.ok ? "ok" : "bad" }, "Automatska provjera: ", auto.ok ? "u redu" : "PROBLEM", " (", auto.checked_by, ")") : null));
  });
  show(h("h1", {}, "Operaterov router"),
    card(h("p", {}, "Custos stoji iza operaterovog routera. Zaštita se ne može zaobići samo ako je ovo sve napravljeno."), items,
      h("div", { class: "row" }, h("button", { type: "button", onclick: () => guarded(async () => {
        const steps = {}; for (const [id] of ISP_STEPS) steps[id] = boxes[id].checked;
        const done = Object.values(steps).every(Boolean);
        await Api.put("/api/wizard", { done, steps }); toast(done ? "Sve gotovo." : "Spremljeno; vratite se kad dovršite.");
        if (done) location.hash = "#pregled";
      }) }, "Spremi"))));
}

// ---- router -------------------------------------------------------------------

const PAGES = { pregled: pagePregled, uredaji: pageUredaji, filtriranje: pageFiltriranje, aktivnost: pageAktivnost, postavke: pagePostavke, wizard: pageWizard, login: pageLogin };

async function route(force) {
  const name = force || location.hash.slice(1) || "pregled";
  const page = PAGES[name] || pagePregled;
  if (page !== pageLogin) document.getElementById("nav").hidden = false;
  for (const a of document.querySelectorAll("nav a")) a.classList.toggle("on", a.getAttribute("href") === "#" + name);
  try { await page(); } catch (e) {
    if (e.status === 401 || e.status === 403) { Api.setCsrf(null); return pageLogin(); }
    show(h("h1", {}, "Greška"), card(h("p", { class: "bad" }, e.message)));
  }
}

document.addEventListener("DOMContentLoaded", () => {
  document.getElementById("logout").addEventListener("click", () => guarded(async () => {
    await Api.post("/api/logout"); Api.setCsrf(null); route("login");
  }));
  window.addEventListener("hashchange", () => route());
  route();
});
