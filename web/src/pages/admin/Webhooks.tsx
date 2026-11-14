import { createResource, createSignal, For, Show } from "solid-js";
import { A } from "@solidjs/router";
import { apiFetch, apiList } from "../../api/client";
import type { Webhook, Workflow } from "../../api/types";
import EventTypePicker from "../../components/integration/EventTypePicker";
import OneTimeSecret from "../../components/integration/OneTimeSecret";
import { validateWebhookForm } from "../../lib/webhook-view";

type Hook = Webhook & { workflowIds?: string[]; hasSecret?: boolean };
type HookForm = { name: string; url: string; secret: string; enabled: boolean; workflowIds: string[]; eventTypes: string[] };
const emptyForm = (): HookForm => ({ name: "", url: "", secret: "", enabled: true, workflowIds: [], eventTypes: [] });

export default function Webhooks() {
  const [hooks, { refetch }] = createResource(() => apiList<Hook>("/admin/api/webhooks"));
  const [workflows] = createResource(() => apiList<Workflow>("/v1/workflows"));
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [form, setForm] = createSignal<HookForm>(emptyForm());
  const [secret, setSecret] = createSignal(""); const [error, setError] = createSignal(""); const [notice, setNotice] = createSignal("");
  const beginEdit = (hook: Hook) => {
    setEditingId(hook.id);
    setForm({ name: hook.name, url: hook.url, secret: "", enabled: hook.enabled, workflowIds: hook.workflowIds ?? [], eventTypes: hook.eventTypes ?? [] });
    setNotice(""); setError("");
  };
  const resetForm = () => { setEditingId(null); setForm(emptyForm()); setError(""); };
  const update = <K extends keyof HookForm>(key: K, value: HookForm[K]) => setForm((current) => ({ ...current, [key]: value }));
  const save = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setNotice("");
    const current = form();
    const errors = validateWebhookForm(current);
    if (Object.keys(errors).length) { setError(Object.values(errors).join(" ")); return; }
    const payload: Record<string, unknown> = { name: current.name.trim(), url: current.url.trim(), enabled: current.enabled, workflowIds: current.workflowIds, eventTypes: current.eventTypes };
    if (current.secret) payload.secret = current.secret;
    try {
      const result = editingId()
        ? await apiFetch<{ secret?: string }>("PATCH", `/admin/api/webhooks/${encodeURIComponent(editingId()!)}`, payload)
        : await apiFetch<{ secret?: string }>("POST", "/admin/api/webhooks", payload);
      if (result?.secret) setSecret(result.secret);
      setNotice(editingId() ? "Webhook updated." : "Webhook created. A test ping was queued."); resetForm(); await refetch();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not save webhook."); }
  };
  const remove = async (hook: Hook) => {
    if (!confirm(`Delete webhook “${hook.name}” and its delivery history?`)) return;
    try { await apiFetch("DELETE", `/admin/api/webhooks/${encodeURIComponent(hook.id)}`); await refetch(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not delete webhook."); }
  };
  const ping = async (hook: Hook) => {
    try { await apiFetch("POST", `/admin/api/webhooks/${encodeURIComponent(hook.id)}/ping`); setNotice("Test delivery queued."); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not queue test delivery."); }
  };
  return <main class="page"><h1>Webhooks</h1><p>Webhook management is a Punchflow development admin feature. New endpoints and URL or secret changes queue a ping.</p>
    <Show when={secret()}><OneTimeSecret value={secret()} onClose={() => setSecret("")} /></Show>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show><Show when={notice()}><p class="success" role="status">{notice()}</p></Show>
    <section class="panel"><h2>{editingId() ? "Edit webhook" : "Add webhook"}</h2>
      <form class="stack-form" onSubmit={save}>
        <label>Name <input required value={form().name} onInput={(event) => update("name", event.currentTarget.value)} /></label>
        <label>Receiver URL <input type="url" required placeholder="http://localhost:9000/hooks" value={form().url} onInput={(event) => update("url", event.currentTarget.value)} /></label>
        <label>Secret {editingId() ? "(leave blank to keep current)" : "(optional)"}<input type="password" autocomplete="new-password" value={form().secret} onInput={(event) => update("secret", event.currentTarget.value)} /></label>
        <label><input type="checkbox" checked={form().enabled} onChange={(event) => update("enabled", event.currentTarget.checked)} /> Enabled</label>
        <label>Workflows <select multiple size="4" value={form().workflowIds} onChange={(event) => update("workflowIds", Array.from(event.currentTarget.selectedOptions, (option) => option.value))}>
          <For each={workflows()?.items ?? []}>{(workflow) => <option value={workflow.id}>{workflow.name}</option>}</For>
        </select></label><p class="muted">Leave none selected to receive events from every workflow.</p>
        <EventTypePicker value={form().eventTypes} onChange={(value) => update("eventTypes", value)} />
        <div class="actions"><button>{editingId() ? "Save changes" : "Create webhook"}</button><Show when={editingId()}><button type="button" class="secondary" onClick={resetForm}>Cancel</button></Show></div>
      </form>
    </section>
    <h2>Registered endpoints</h2><Show when={!hooks.loading} fallback={<p>Loading webhooks…</p>}><div class="table-wrap"><table><thead><tr><th>Name</th><th>URL</th><th>Events</th><th>Status</th><th>Actions</th></tr></thead><tbody>
      <For each={hooks()?.items ?? []}>{(hook) => <tr><td>{hook.name}</td><td><code>{hook.url}</code><div class="muted">{hook.hasSecret ? "Secret configured" : "No secret"}</div></td><td>{hook.eventTypes?.length ? hook.eventTypes.join(", ") : "All events"}</td><td>{hook.enabled ? "Enabled" : "Disabled"}</td><td class="actions"><button onClick={() => beginEdit(hook)}>Edit</button><button onClick={() => ping(hook)}>Send test</button><A href={`/admin/webhooks/${encodeURIComponent(hook.id)}/events`}>Events</A><button class="danger" onClick={() => remove(hook)}>Delete</button></td></tr>}</For>
    </tbody></table><Show when={(hooks()?.items.length ?? 0) === 0}><p class="muted">No webhook endpoints configured.</p></Show></div></Show>
  </main>;
}
