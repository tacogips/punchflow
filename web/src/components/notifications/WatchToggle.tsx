import { Show, createResource, createSignal } from "solid-js";
import { apiFetch } from "../../api/client";
import type { WatchState } from "../../api/ui-types";
import "./notifications.css";

/** Watch / unwatch button for one ticket. Watchers receive activity notifications in the inbox. */
export default function WatchToggle(props: { ticketId: string }) {
  const [state, { mutate }] = createResource(() => props.ticketId, (id) => apiFetch<WatchState>("GET", `/ui/api/tickets/${id}/watch`));
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const toggle = async () => {
    const current = state();
    if (!current) return;
    setBusy(true); setError("");
    try {
      mutate(await apiFetch<WatchState>(current.watching ? "DELETE" : "POST", `/ui/api/tickets/${props.ticketId}/watch`));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to update watch");
    } finally { setBusy(false); }
  };
  return <Show when={state()}>{(current) => <span class="watch-toggle">
    <button type="button" aria-pressed={current().watching} disabled={busy()} onClick={toggle}>{current().watching ? "Unwatch" : "Watch"}</button>
    <Show when={current().watching && current().source === "workflow"}><small>watching through the workflow</small></Show>
    <Show when={current().watching && current().source === "applicant"}><small>watching as the applicant</small></Show>
    <Show when={current().watching && current().source === "approver"}><small>watching as an approver</small></Show>
    <Show when={error()}><small role="alert">{error()}</small></Show>
  </span>}</Show>;
}
