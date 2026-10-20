import { For, Show, createResource, createSignal } from "solid-js";
import { A, useParams } from "@solidjs/router";
import type { TicketDetail as TicketDetailType, TicketWithStep } from "../../api/types";
import { apiFetch } from "../../api/client";
import { session } from "../../session";
import { formatJst } from "../../lib/format";
import { attachmentsOf, inputLabel, inputValueText, userLabel } from "../../lib/ticket-view";
import ActionBar from "../../components/tickets/ActionBar";
import Comments from "../../components/tickets/Comments";
import StepList from "../../components/tickets/StepList";
import Viewers from "../../components/tickets/Viewers";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";
function TicketContent(props: {
  ticket: TicketDetailType;
  links: TicketWithStep[];
  busy: boolean;
  error: string;
  onAction: (name: "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit", data?: { comment?: string; to?: number }) => void;
  onChanged: () => void;
}) {
  const t = props.ticket; const workflow = obj(t.workflow); const author = obj(t.author); const currentSession = session();
  return <>
    <header class="page-header"><div><h1>{str(t.ticketNumber) || str(t.title) || "Ticket"}</h1><p>{str(t.title)} · {str(workflow.name)}</p></div><span class={`status status-${t.status}`}>{t.status.replaceAll("_", " ")}</span></header>
    <Show when={props.error}><p role="alert">{props.error}</p></Show>
    <dl class="ticket-metadata"><dt>Author</dt><dd>{userLabel(author) || "Unknown"}</dd><dt>Created</dt><dd>{formatJst(str(t.createdAt) || null)}</dd><dt>Updated</dt><dd>{formatJst(str(t.updatedAt) || null)}</dd><dt>Opened</dt><dd>{formatJst(str(t.openedAt) || null)}</dd><dt>Current step</dt><dd>{String(t.currentStep ?? "—")}</dd></dl>
    <Show when={currentSession}><ActionBar ticket={t} me={{ id: currentSession!.user.id, permissions: currentSession!.permissions }} busy={props.busy} onAction={props.onAction} /></Show>
    <section><h2>Form inputs</h2><dl class="ticket-inputs"><For each={arr(t.inputs)}>{(input) => <><dt>{inputLabel(input)}</dt><dd>{inputValueText(input)}<Show when={attachmentsOf(input).length}><ul><For each={attachmentsOf(input)}>{(file) => <li><a href={file.url}>{file.filename}</a></li>}</For></ul></Show></dd></>}</For></dl>
      <For each={arr(t.slipItems)}>{(slip, index) => <section><h3>{str(obj(slip).title) || `Slip row ${index() + 1}`}</h3><dl><For each={arr(obj(slip).inputs)}>{(input) => <><dt>{inputLabel(input)}</dt><dd>{inputValueText(input)}<Show when={attachmentsOf(input).length}><ul><For each={attachmentsOf(input)}>{(file) => <li><a href={file.url}>{file.filename}</a></li>}</For></ul></Show></dd></>}</For></dl></section>}</For>
    </section>
    <section><h2>Approval steps</h2><StepList steps={t.steps ?? []} currentStep={Number(t.currentStep ?? 0)} /></section>
    <Show when={props.links.length}><section><h2>Related tickets</h2><ul><For each={props.links}>{(link) => <li><A href={`/tickets/${link.id}`}>{link.ticketNumber || link.title || link.id}</A></li>}</For></ul></section></Show>
    <Comments ticketId={t.id} enabled={workflow.commentingEnabled !== false} onChanged={props.onChanged} />
    <Viewers ticketId={t.id} onChanged={props.onChanged} />
  </>;
}

export default function TicketDetail() {
  const params = useParams(); const [ticket, { refetch }] = createResource(() => params.id, (id) => apiFetch<TicketDetailType>("GET", `/v1/tickets/${id}`));
  const [links] = createResource(() => params.id, (id) => apiFetch<TicketWithStep[]>("GET", `/v1/tickets/${id}/links`));
  const [busy, setBusy] = createSignal(false); const [error, setError] = createSignal("");
  const refresh = async () => { await refetch(); };
  const action = async (name: "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit", data?: { comment?: string; to?: number }) => {
    if (name === "edit") return;
    const body: Record<string, unknown> = {};
    if (data?.comment) body.comment = data.comment;
    if (name === "reject") body.to = data?.to ?? 0;
    setBusy(true); setError("");
    try { await apiFetch("POST", `/v1/tickets/${params.id}/${name}`, Object.keys(body).length ? body : undefined); await refresh(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to update ticket"); }
    finally { setBusy(false); }
  };
  const detail = () => ticket();
  return <main class="page"><A href="/tickets">Back to tickets</A>
    <Show when={!ticket.loading} fallback={<p>Loading ticket…</p>}><Show when={!ticket.error && detail()} fallback={<p role="alert">Unable to load ticket.</p>}>
      <TicketContent ticket={detail()!} links={links() ?? []} busy={busy()} error={error()} onAction={action} onChanged={refresh} />
    </Show></Show>
  </main>;
}
