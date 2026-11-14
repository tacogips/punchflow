import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList, KickflowError } from "../../api/client";

type Tenant = { name: string; publicId: string; plan: string; mode: string; ipAllowlist: string[]; allowedEmailDomains: string[]; selfServiceProxy: boolean };
type Fault = { id: string; method?: string; pathPrefix: string; status: number; code: string; message?: string; remaining?: number };
const emptyTenant: Tenant = { name: "", publicId: "", plan: "enterprise", mode: "normal", ipAllowlist: [], allowedEmailDomains: [], selfServiceProxy: false };

export default function DevTools() {
  const [tenant, { refetch: reloadTenant }] = createResource(() => apiFetch<Tenant>("GET", "/admin/api/tenant"));
  const [faults, { refetch: reloadFaults }] = createResource(() => apiList<Fault>("/admin/api/faults"));
  const [settings, setSettings] = createSignal<Tenant>(emptyTenant);
  const [fault, setFault] = createSignal({ method: "", pathPrefix: "/v1/", status: 500, code: "internal_server_error", message: "", remaining: 1 });
  const [error, setError] = createSignal(""); const [notice, setNotice] = createSignal("");
  const applyTenant = (value: Tenant | null | undefined) => { if (value) setSettings({ ...emptyTenant, ...value }); };
  createEffect(() => applyTenant(tenant()));
  const saveTenant = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setNotice("");
    try { await apiFetch("PATCH", "/admin/api/tenant", settings()); setNotice("Tenant settings saved."); applyTenant(await reloadTenant()); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not save tenant settings."); }
  };
  const addFault = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setNotice("");
    const value = fault();
    const payload: Record<string, unknown> = { pathPrefix: value.pathPrefix, status: Number(value.status), code: value.code, remaining: Number(value.remaining) };
    if (value.method.trim()) payload.method = value.method.trim().toUpperCase();
    if (value.message.trim()) payload.message = value.message.trim();
    try { await apiFetch("POST", "/admin/api/faults", payload); setNotice("Fault rule added."); setFault({ ...value, message: "" }); await reloadFaults(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not add fault rule."); }
  };
  const removeFault = async (id: string) => {
    try { await apiFetch("DELETE", `/admin/api/faults/${encodeURIComponent(id)}`); await reloadFaults(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not delete fault rule."); }
  };
  const exportFixtures = async () => {
    try {
      const contents = await apiFetch<Record<string, unknown>>("GET", "/admin/api/fixtures/export");
      const url = URL.createObjectURL(new Blob([JSON.stringify(contents, null, 2)], { type: "application/json" }));
      const link = document.createElement("a"); link.href = url; link.download = "punchflow-fixtures.json"; link.click(); URL.revokeObjectURL(url);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not export fixtures."); }
  };
  const resetFixtures = async () => {
    if (!confirm("Reset all database data to the configured fixtures? This action cannot be undone.")) return;
    setError(""); setNotice("");
    try { const result = await apiFetch<{ message?: string }>("POST", "/admin/api/fixtures/reset"); setNotice(result?.message ?? "Database reset to fixtures."); await reloadTenant(); await reloadFaults(); }
    catch (cause) { setError(cause instanceof KickflowError && cause.status === 404 ? "Fixture reset is disabled on this server." : cause instanceof Error ? cause.message : "Could not reset database."); }
  };
  const setSetting = <K extends keyof Tenant>(key: K, value: Tenant[K]) => setSettings((current) => ({ ...current, [key]: value }));
  return <main class="page"><h1>Developer Tools</h1><p>These controls change local development data and server behavior.</p>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show><Show when={notice()}><p class="success" role="status">{notice()}</p></Show>
    <section class="panel"><h2>Tenant settings</h2><Show when={!tenant.loading} fallback={<p>Loading settings…</p>}><form class="stack-form" onSubmit={saveTenant}>
      <label>Tenant name <input value={settings().name} onInput={(event) => setSetting("name", event.currentTarget.value)} /></label>
      <label>Public ID <input value={settings().publicId} onInput={(event) => setSetting("publicId", event.currentTarget.value)} /></label>
      <label>Plan <select value={settings().plan} onChange={(event) => setSetting("plan", event.currentTarget.value)}><option>enterprise</option><option>standard</option></select></label>
      <label>Mode <select value={settings().mode} onChange={(event) => setSetting("mode", event.currentTarget.value)}><option>normal</option><option>suspended</option><option>maintenance</option><option>admin_working</option></select></label>
      <label>IP allowlist (one address per line) <textarea value={settings().ipAllowlist.join("\n")} onInput={(event) => setSetting("ipAllowlist", event.currentTarget.value.split("\n").map((v) => v.trim()).filter(Boolean))} /></label>
      <label>Allowed email domains (one per line) <textarea value={settings().allowedEmailDomains.join("\n")} onInput={(event) => setSetting("allowedEmailDomains", event.currentTarget.value.split("\n").map((v) => v.trim()).filter(Boolean))} /></label>
      <label><input type="checkbox" checked={settings().selfServiceProxy} onChange={(event) => setSetting("selfServiceProxy", event.currentTarget.checked)} /> Allow self-service proxy</label>
      <button>Save tenant settings</button>
    </form></Show></section>
    <section class="panel"><h2>Fault injection</h2><p>Rules affect matching <code>/v1</code> requests until the count expires or the rule is removed. Set remaining to 0 for no limit.</p>
      <form class="inline-form" onSubmit={addFault}>
        <label>Method (optional) <input placeholder="GET" value={fault().method} onInput={(event) => setFault({ ...fault(), method: event.currentTarget.value })} /></label>
        <label>Path prefix <input required value={fault().pathPrefix} onInput={(event) => setFault({ ...fault(), pathPrefix: event.currentTarget.value })} /></label>
        <label>Status <input type="number" min="400" max="599" value={fault().status} onInput={(event) => setFault({ ...fault(), status: Number(event.currentTarget.value) })} /></label>
        <label>Error code <input required value={fault().code} onInput={(event) => setFault({ ...fault(), code: event.currentTarget.value })} /></label>
        <label>Message <input value={fault().message} onInput={(event) => setFault({ ...fault(), message: event.currentTarget.value })} /></label>
        <label>Remaining <input type="number" min="0" value={fault().remaining} onInput={(event) => setFault({ ...fault(), remaining: Number(event.currentTarget.value) })} /></label><button>Add rule</button>
      </form>
      <div class="table-wrap"><table><thead><tr><th>Method</th><th>Path</th><th>Response</th><th>Remaining</th><th></th></tr></thead><tbody><For each={faults()?.items ?? []}>{(rule) => <tr><td>{rule.method ?? "Any"}</td><td><code>{rule.pathPrefix}</code></td><td>{rule.status} {rule.code}</td><td>{rule.remaining === 0 ? "Unlimited" : rule.remaining ?? "—"}</td><td><button class="danger" onClick={() => removeFault(rule.id)}>Delete</button></td></tr>}</For></tbody></table><Show when={!faults.loading && (faults()?.items.length ?? 0) === 0}><p class="muted">No active fault rules.</p></Show></div>
    </section>
    <section class="panel"><h2>Fixture data</h2><div class="actions"><button onClick={exportFixtures}>Download fixture export</button><button class="danger" onClick={resetFixtures}>Reset to fixtures</button></div><p class="muted">Reset availability depends on the server's explicit admin reset setting.</p></section>
  </main>;
}
