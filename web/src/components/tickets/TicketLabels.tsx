import { For, Show, createResource, createSignal } from "solid-js";
import type { Label } from "../../api/types";
import { apiFetch } from "../../api/client";
import { attachableLabels, labelTextColor, uiTicketPath } from "../../lib/ticket-controls";

/** Labels of one ticket. Any user who can view the ticket may attach or detach them. */
export default function TicketLabels(props: { ticketId: string; labels: Label[]; onChanged: () => void | Promise<void> }) {
  const [catalog] = createResource(() => apiFetch<Label[]>("GET", "/ui/api/labels").catch(() => [] as Label[]));
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [choice, setChoice] = createSignal("");
  const options = () => attachableLabels(catalog() ?? [], props.labels);
  const run = async (request: () => Promise<unknown>) => {
    setBusy(true); setError("");
    try { await request(); await props.onChanged(); setChoice(""); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to change labels"); }
    finally { setBusy(false); }
  };
  const attach = () => { const labelId = choice(); if (labelId) void run(() => apiFetch("POST", uiTicketPath(props.ticketId, "/labels"), { labelId })); };
  const detach = (label: Label) => void run(() => apiFetch("DELETE", uiTicketPath(props.ticketId, `/labels/${encodeURIComponent(label.id)}`)));
  return <section class="ticket-labels" aria-label="Labels"><h2>Labels</h2>
    <Show when={error()}><p role="alert">{error()}</p></Show>
    <ul class="label-list">
      <For each={props.labels} fallback={<li class="muted">No labels</li>}>{(label) =>
        <li><span class="label-chip" title={label.description ?? ""} style={{ "background-color": `#${label.color}`, color: labelTextColor(label.color) }}>{label.name}</span>
          <button type="button" aria-label={`Remove label ${label.name}`} disabled={busy()} onClick={() => detach(label)}>Remove</button></li>}</For>
    </ul>
    <Show when={options().length}>
      <label>Add label <select value={choice()} onChange={(event) => setChoice(event.currentTarget.value)}><option value="">Select…</option><For each={options()}>{(label) => <option value={label.id}>{label.name}</option>}</For></select></label>
      <button type="button" disabled={busy() || !choice()} onClick={attach}>Add label</button>
    </Show>
  </section>;
}
