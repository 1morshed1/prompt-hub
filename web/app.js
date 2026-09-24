/* PromptHub dashboard — a ~zero-dependency single-page app over the JSON API.
 *
 * It's intentionally framework-free: a hash router picks a view function, each
 * view fetches from the API and returns DOM. If you can read fetch() and template
 * literals, you can read this. See docs/learning/08-dashboard.md.
 */

// ─── tiny helpers ────────────────────────────────────────────────────────────

const app = document.getElementById("app");

// api(path, opts) -> parsed JSON, throwing a readable Error on non-2xx.
async function api(path, opts = {}) {
  const res = await fetch("/api/v1" + path, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : {};
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

// h(tag, attrs, ...children) — minimal hyperscript. attrs.html sets innerHTML;
// function-valued attrs (onclick) become listeners.
function h(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "html") e.innerHTML = v;
    else if (k === "class") e.className = v;
    else if (typeof v === "function") e.addEventListener(k.replace(/^on/, ""), v);
    else if (v !== null && v !== undefined) e.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c == null) continue;
    e.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return e;
}

function render(...nodes) {
  app.replaceChildren(...nodes.flat().filter(Boolean));
}

function errorCard(err) {
  return h("div", { class: "error" }, "Error: " + err.message);
}

function fmt(n, digits = 2) {
  if (n === null || n === undefined) return "—";
  return Number(n).toFixed(digits);
}

// ─── router ──────────────────────────────────────────────────────────────────

const routes = [
  [/^#\/$/, () => promptsView()],
  [/^#\/prompt\/(.+)$/, (m) => promptDetailView(decodeURIComponent(m[1]))],
  [/^#\/experiments$/, () => experimentsView()],
  [/^#\/experiment\/(.+)$/, (m) => experimentReportView(m[1])],
];

async function route() {
  const hash = location.hash || "#/";
  for (const [re, view] of routes) {
    const m = hash.match(re);
    if (m) {
      app.replaceChildren(h("p", { class: "muted" }, "Loading…"));
      try {
        await view(m);
      } catch (err) {
        render(errorCard(err));
      }
      return;
    }
  }
  render(h("p", {}, "Not found."));
}

window.addEventListener("hashchange", route);
window.addEventListener("DOMContentLoaded", route);

// ─── view: prompts list ──────────────────────────────────────────────────────

async function promptsView() {
  const { prompts } = await api("/prompts");
  const list = (prompts || []).map((p) =>
    h("a", { class: "card list-item", href: `#/prompt/${encodeURIComponent(p.name)}` },
      h("div", { class: "spread" },
        h("div", {},
          h("strong", {}, p.name),
          h("div", { class: "muted small" }, p.description || "No description")),
        h("span", { class: "badge" }, "project: " + p.project))));

  render(
    h("div", { class: "spread" },
      h("h1", {}, "Prompts"),
      h("button", { onclick: () => toggleCreatePrompt() }, "+ New prompt")),
    h("div", { id: "create-prompt" }),
    list.length ? h("div", {}, list) : h("p", { class: "muted" }, "No prompts yet. Create one to get started."),
  );
}

function toggleCreatePrompt() {
  const slot = document.getElementById("create-prompt");
  if (slot.childElementCount) { slot.replaceChildren(); return; }
  slot.replaceChildren(h("div", { class: "card" },
    h("label", {}, "Name"), h("input", { id: "np-name", placeholder: "order-assistant" }),
    h("label", {}, "Description"), h("input", { id: "np-desc", placeholder: "helps customers with orders" }),
    h("div", { class: "row", style: "margin-top:12px" },
      h("button", { onclick: async () => {
        try {
          const name = document.getElementById("np-name").value.trim();
          if (!name) return;
          await api("/prompts", { method: "POST", body: JSON.stringify({
            name, description: document.getElementById("np-desc").value }) });
          location.hash = `#/prompt/${encodeURIComponent(name)}`;
        } catch (e) { alert(e.message); }
      } }, "Create"),
      h("button", { class: "secondary", onclick: toggleCreatePrompt }, "Cancel"))));
}

// ─── view: prompt detail ─────────────────────────────────────────────────────

async function promptDetailView(name) {
  const p = await api(`/prompts/${encodeURIComponent(name)}`);
  const analytics = await api(`/prompts/${encodeURIComponent(name)}/analytics`).catch(() => ({ versions: [] }));
  const versions = p.versions || [];
  const tags = p.tags || [];
  // Map version_id -> which tags point at it, for the timeline.
  const tagsByVersion = {};
  for (const t of tags) (tagsByVersion[t.version_id] ??= []).push(t.tag);
  const analyticsByVersion = {};
  for (const a of analytics.versions || []) analyticsByVersion[a.version_number] = a;

  render(
    h("div", { class: "spread" },
      h("div", {}, h("a", { href: "#/", class: "muted small" }, "← all prompts"),
        h("h1", {}, p.name), h("div", { class: "muted" }, p.description || "")),
      h("div", { class: "row" },
        h("button", { onclick: () => toggleNewVersion(name) }, "+ New version"))),

    h("div", { id: "new-version" }),

    h("h2", {}, "Tags"),
    tags.length
      ? h("div", { class: "row" }, tags.map((t) => {
          const v = versions.find((x) => x.id === t.version_id);
          return h("span", { class: "badge tag" }, `${t.tag} → v${v ? v.version_number : "?"}`);
        }))
      : h("p", { class: "muted small" }, "No tags. Tag a version below to deploy it."),

    h("h2", {}, "Version history"),
    versionTimeline(name, versions, tagsByVersion, analyticsByVersion),

    h("h2", {}, "Compare versions"),
    diffControls(name, versions),
    h("div", { id: "diff-out" }),
  );
}

function versionTimeline(name, versions, tagsByVersion, analyticsByVersion) {
  if (!versions.length) return h("p", { class: "muted" }, "No versions yet.");
  return h("div", {}, versions.map((v) => {
    const vtags = tagsByVersion[v.id] || [];
    const a = analyticsByVersion[v.version_number];
    return h("div", { class: "card" },
      h("div", { class: "spread" },
        h("div", { class: "row" },
          h("span", { class: "badge" }, "v" + v.version_number),
          ...vtags.map((t) => h("span", { class: "badge tag" }, t)),
          h("span", { class: "muted small" }, v.commit_message || "(no message)")),
        h("button", { class: "secondary", onclick: () => promptSetTag(name, v.version_number) }, "Set tag…")),
      h("details", { style: "margin-top:8px" },
        h("summary", { class: "small muted" }, "templates & config"),
        h("h3", {}, "System"), h("pre", { class: "diff" }, h("div", { class: "seg equal" }, v.system_template || "(empty)")),
        h("h3", {}, "User"), h("pre", { class: "diff" }, h("div", { class: "seg equal" }, v.user_template || "(empty)")),
        h("h3", {}, "Model config"), h("div", { class: "mono" }, JSON.stringify(v.model_config || {})),
      ),
      a ? h("div", { class: "row small muted", style: "margin-top:8px" },
            h("span", {}, `${a.renders} renders`),
            h("span", {}, `avg rating: ${a.avg_rating == null ? "—" : fmt(a.avg_rating * 100, 1) + "%"}`),
            h("span", {}, `avg latency: ${a.avg_latency_ms == null ? "—" : fmt(a.avg_latency_ms, 0) + "ms"}`),
            h("span", {}, `tokens: ${a.total_tokens}`))
        : null);
  }));
}

function diffControls(name, versions) {
  if (versions.length < 2) return h("p", { class: "muted small" }, "Need at least two versions to diff.");
  const opts = versions.map((v) => h("option", { value: v.version_number }, "v" + v.version_number));
  const from = h("select", {}, ...versions.map((v) => h("option", { value: v.version_number }, "v" + v.version_number)));
  const to = h("select", {}, ...opts);
  from.value = versions[1].version_number; // older
  to.value = versions[0].version_number;   // newer
  const mode = h("select", {}, h("option", { value: "line" }, "line"), h("option", { value: "token" }, "token"));
  return h("div", { class: "card row" },
    h("span", {}, "from"), from, h("span", {}, "to"), to, mode,
    h("button", { onclick: async () => {
      try {
        const d = await api(`/prompts/${encodeURIComponent(name)}/diff?from=${from.value}&to=${to.value}&mode=${mode.value}`);
        document.getElementById("diff-out").replaceChildren(renderDiff(d));
      } catch (e) { document.getElementById("diff-out").replaceChildren(errorCard(e)); }
    } }, "Diff"));
}

function renderDiff(d) {
  const block = (title, part) => h("div", {},
    h("h3", {}, title + `  (+${part.stats.added} −${part.stats.removed})`),
    h("pre", { class: "diff" }, part.segments.map((s) =>
      h("span", { class: "seg " + s.op }, (s.op === "insert" ? "+ " : s.op === "delete" ? "- " : "  ") + s.text + "\n"))));
  return h("div", {}, block("System template", d.system_template), block("User template", d.user_template));
}

async function promptSetTag(name, versionNumber) {
  const tag = prompt(`Point which tag at v${versionNumber}? (e.g. production, staging)`, "production");
  if (!tag) return;
  try {
    await api(`/prompts/${encodeURIComponent(name)}/tags/${encodeURIComponent(tag)}`, {
      method: "PUT", body: JSON.stringify({ version_number: versionNumber, updated_by: "dashboard" }) });
    route(); // refresh
  } catch (e) { alert(e.message); }
}

function toggleNewVersion(name) {
  const slot = document.getElementById("new-version");
  if (slot.childElementCount) { slot.replaceChildren(); return; }
  slot.replaceChildren(h("div", { class: "card" },
    h("label", {}, "System template ", h("span", { class: "field-hint" }, "— supports {{vars}}, {% if %}, {% for %}")),
    h("textarea", { id: "nv-sys", placeholder: "You are a {{role}} assistant." }),
    h("label", {}, "User template"),
    h("textarea", { id: "nv-user", placeholder: "About {{topic}}: {{query}}" }),
    h("label", {}, "Variables (JSON array)"),
    h("textarea", { id: "nv-vars", placeholder: '[{"name":"role","type":"string","required":true}]' }),
    h("label", {}, "Model config (JSON)"),
    h("textarea", { id: "nv-model", placeholder: '{"model":"gpt-4o","temperature":0.2}' }),
    h("label", {}, "Commit message"),
    h("input", { id: "nv-msg", placeholder: "Improved refund tone" }),
    h("div", { class: "row", style: "margin-top:12px" },
      h("button", { onclick: async () => {
        try {
          const body = {
            system_template: document.getElementById("nv-sys").value,
            user_template: document.getElementById("nv-user").value,
            variables: parseJSONOr(document.getElementById("nv-vars").value, []),
            model_config: parseJSONOr(document.getElementById("nv-model").value, {}),
            commit_message: document.getElementById("nv-msg").value,
          };
          await api(`/prompts/${encodeURIComponent(name)}/versions`, { method: "POST", body: JSON.stringify(body) });
          route();
        } catch (e) { alert(e.message); }
      } }, "Create version"),
      h("button", { class: "secondary", onclick: () => toggleNewVersion(name) }, "Cancel"))));
}

function parseJSONOr(text, fallback) {
  text = (text || "").trim();
  if (!text) return fallback;
  return JSON.parse(text);
}

// ─── view: experiments list ──────────────────────────────────────────────────

async function experimentsView() {
  const { experiments } = await api("/experiments");
  const statusBadge = (s) => h("span", { class: "badge " + (s === "running" ? "green" : s === "promoted" ? "amber" : "") }, s);
  const list = (experiments || []).map((e) =>
    h("a", { class: "card list-item", href: `#/experiment/${e.id}` },
      h("div", { class: "spread" },
        h("div", {}, h("strong", {}, e.name || "(unnamed experiment)"),
          h("div", { class: "muted small mono" }, e.id)),
        h("div", { class: "row" },
          h("span", { class: "badge tag" }, "split " + fmt(e.traffic_split, 2)),
          e.auto_promote ? h("span", { class: "badge" }, "auto-promote") : null,
          statusBadge(e.status)))));
  render(
    h("h1", {}, "Experiments"),
    list.length ? h("div", {}, list) : h("p", { class: "muted" }, "No experiments yet. Create one from the API or SDK (see docs)."),
  );
}

// ─── view: experiment report (side-by-side comparison) ───────────────────────

async function experimentReportView(id) {
  const { report, promotion } = await api(`/experiments/${id}/report`);
  const r = report;

  const winnerClass = r.winner === "variant" ? "green" : r.winner === "control" ? "amber" : "";
  const banner = h("div", { class: "winner-banner badge " + winnerClass, style: "display:block" },
    r.winner === "no_winner" ? "No significant winner yet" : `Winner: ${r.winner}`);

  const armCard = (title, arm, highlight) => h("div", { class: "card" },
    h("div", { class: "spread" }, h("h3", { style: "margin:0" }, title),
      highlight ? h("span", { class: "badge green" }, "winner") : null),
    metric("Samples", arm.sample_size),
    metric("Rated", arm.rated_count),
    metric("👍 up-rate", fmt(arm.avg_rating * 100, 1) + "%"),
    ratingBar(arm.avg_rating),
    metric("Avg latency", fmt(arm.avg_latency_ms, 0) + " ms"),
    metric("Avg cost", "$" + fmt(arm.avg_cost_usd, 6)));

  render(
    h("a", { href: "#/experiments", class: "muted small" }, "← experiments"),
    h("h1", {}, "Experiment report"),
    banner,
    h("div", { class: "notice" }, r.recommendation),
    h("div", { class: "compare" },
      armCard("Control", r.control, r.winner === "control"),
      armCard("Variant", r.variant, r.winner === "variant")),

    h("h2", {}, "Statistical significance"),
    h("div", { class: "card" },
      metric("Rating test (χ²)", `χ²=${fmt(r.rating_chi_squared)} · p=${fmtP(r.rating_p_value)} · ${sig(r.rating_significant)}`),
      metric("Latency test (Mann-Whitney U)", `p=${fmtP(r.latency_p_value)} · ${sig(r.latency_significant)}`),
      h("p", { class: "muted small" }, "Ratings use a chi-squared test; latency uses the non-parametric Mann-Whitney U. Significance threshold α = 0.05.")),

    promotion ? h("div", { class: "card" },
      h("h3", {}, "Auto-promotion"),
      h("div", { class: promotion.promoted ? "notice" : "muted small" }, promotion.reason)) : null,
  );
}

function metric(label, value) {
  return h("div", { class: "metric" }, h("span", { class: "muted" }, label), h("b", {}, String(value)));
}
function ratingBar(avg) {
  const pct = Math.max(0, Math.min(100, (avg || 0) * 100));
  return h("div", { class: "barwrap", style: "margin:4px 0 8px" }, h("div", { class: "bar", style: `width:${pct}%` }));
}
function fmtP(p) { return p < 0.0001 ? p.toExponential(2) : fmt(p, 4); }
function sig(b) { return b ? "significant" : "not significant"; }
