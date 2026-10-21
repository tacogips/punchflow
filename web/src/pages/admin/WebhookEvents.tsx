import { createResource, createSignal, For, Show } from "solid-js";
import { A, useParams } from "@solidjs/router";
import { apiFetch, apiList } from "../../api/client";
import type { WebhookEvent } from "../../api/types";
import AttemptList from "../../components/integration/AttemptList";
import { deliveryStatusLabel, WEBHOOK_EVENT_TYPES } from "../../lib/webhook-view";

type EventRow = WebhookEvent & { status: string; attemptCount: number; lastStatusCode?: number | null; nextAttemptAt: string | null; createdAt?: string };
type EventDetail = EventRow & { attempts: Array<{ id: string; statusCode?: number | null; startedAt?: string; durationMs?: number; error?: string | null; responseBody?: string | null; manual?: boolean }> };

export default function WebhookEvents() {
  const params = useParams<{ id: string }>(); const webhookId = () => params.id;
  const [page, setPage] = createSignal(1); const [events, { refetch }] = createResource(() => [webhookId(), page()] as const, ([id, currentPage]) => apiList<EventRow>(`/admin/api/webhooks/${encodeURIComponent(id)}/events`, { page: String(currentPage), perPage: "25" }));
  const [selectedId, setSelectedId] = createSignal("");
  const [detail] = createResource(selectedId, (id) => id ? apiFetch<EventDetail>("GET", `/admin/api/webhook-events/${encodeURIComponent(id)}`) : undefined);
  const [eventType, setEventType] = createSignal<string>(WEBHOOK_EVENT_TYPES[0]);
  const [ticketId, setTicketId] = createSignal(""); const [commentId, setCommentId] = createSignal("");
  const [error, setError] = createSignal(""); const [notice, setNotice] = createSignal("");
  const resend = async (id: string) => {
    try { await apiFetch("POST", `/admin/api/webhook-events/${encodeURIComponent(id)}/resend`); setNotice("Manual resend queued."); await refetch(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not resend event."); }
  };
  const fire = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setNotice("");
    const payload: Record<string, unknown> = { eventType: eventType(), webhookIds: [webhookId()] };
    if (ticketId().trim()) payload.ticketId = ticketId().trim();
    if (commentId().trim()) payload.commentId = commentId().trim();
    try {
      await apiFetch("POST", "/admin/api/webhook-events", payload);
      setTicketId(""); setCommentId(""); setNotice("Event queued for delivery."); await refetch();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not fire event."); }
  };
  return <main class="page"><p><A href="/admin/webhooks">Back to webhooks</A></p><h1>Webhook events</h1>
    <p>Delivery history for endpoint <code>{webhookId()}</code>.</p>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show><Show when={notice()}><p class="success" role="status">{notice()}</p></Show>
    <section class="panel"><h2>Fire a test event</h2><form class="inline-form" onSubmit={fire}>
      <label>Event type <select value={eventType()} onChange={(event) => setEventType(event.currentTarget.value)}><For each={WEBHOOK_EVENT_TYPES}>{(type) => <option value={type}>{type}</option>}</For></select></label>
      <label>Ticket ID <input value={ticketId()} onInput={(event) => setTicketId(event.currentTarget.value)} /></label>
      <label>Comment ID <input value={commentId()} onInput={(event) => setCommentId(event.currentTarget.value)} /></label>
      <button>Queue event</button>
    </form><p class="muted">Ticket events need a ticket ID. Comment events also need a comment ID.</p></section>
    <Show when={!events.loading} fallback={<p>Loading deliveries…</p>}><div class="table-wrap"><table><thead><tr><th>Event</th><th>Delivery ID</th><th>Status</th><th>Attempts</th><th>Last HTTP</th><th>Next attempt</th><th></th></tr></thead><tbody>
      <For each={events()?.items ?? []}>{(item) => <tr><td>{item.eventType}</td><td><code>{item.id}</code></td><td>{deliveryStatusLabel(item)}</td><td>{item.attemptCount}</td><td>{item.lastStatusCode ?? "—"}</td><td>{item.nextAttemptAt ?? "—"}</td><td><button onClick={() => setSelectedId(selectedId() === item.id ? "" : item.id)}>{selectedId() === item.id ? "Hide" : "Details"}</button><button onClick={() => resend(item.id)}>Resend</button></td></tr>}</For>
    </tbody></table><Show when={(events()?.items.length ?? 0) === 0}><p class="muted">No delivery events yet.</p></Show>
      <div class="pagination"><button disabled={page() <= 1} onClick={() => setPage(page() - 1)}>Previous</button><span>Page {events()?.pagination.page ?? page()} · {events()?.pagination.total ?? 0} total</span><button disabled={!events()?.pagination.links.next} onClick={() => setPage(page() + 1)}>Next</button></div>
    </div></Show>
    <Show when={selectedId()}><section class="panel"><h2>Delivery attempts</h2><Show when={!detail.loading} fallback={<p>Loading event…</p>}><Show when={detail()} fallback={<p>No event details available.</p>}><AttemptList attempts={detail()?.attempts ?? []} /></Show></Show></section></Show>
  </main>;
}
