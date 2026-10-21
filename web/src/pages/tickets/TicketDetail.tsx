import { For, Show, createResource, createSignal } from "solid-js";
import { A, useParams } from "@solidjs/router";
import type { CustomStepBody, Label, SubStatus, TicketControls, TicketDetail as TicketDetailType, TicketWithStep } from "../../api/types";
import { apiFetch } from "../../api/client";
import { session } from "../../session";
import { formatJst } from "../../lib/format";
import { attachmentsOf, inputLabel, inputValueText, userLabel } from "../../lib/ticket-view";
import { NO_CONTROLS, normalizeControls, subStatusRestrictions, uiTicketPath } from "../../lib/ticket-controls";
import ActionBar from "../../components/tickets/ActionBar";
import ControlsBar from "../../components/tickets/ControlsBar";
import TicketLabels from "../../components/tickets/TicketLabels";
import Comments from "../../components/tickets/Comments";
import StepList from "../../components/tickets/StepList";
import WatchToggle from "../../components/notifications/WatchToggle";
import Viewers from "../../components/tickets/Viewers";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";
function TicketContent(props: {
  ticket: TicketDetailType;
  links: TicketWithStep[];
  busy: boolean;
  error: string;
  controls: TicketControls;
  onCancelApproval: (comment?: string) => void;
  onRaise: (to?: number) => void;
  onAddStep: (body: CustomStepBody) => void;
  onRemoveStep: (stepId: string) => void;
  onAssignCurrentApprover: (ticketStepId: string, userId: string) => void;
  onSetHold: (assigneeId: string, pending: boolean) => void;
  onAction: (name: "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit", data?: { comment?: string; to?: number }) => void;
  onChanged: () => void;
}) {
  const t = props.ticket; const workflow = obj(t.workflow); const subStatus = () => (t.subStatus ?? null) as SubStatus | null; const labels = () => (Array.isArray(t.labels) ? t.labels : []) as Label[]; const author = obj(t.author); const currentSession = session(); const [replacementUserId, setReplacementUserId] = createSignal(""); const [replacementStepId, setReplacementStepId] = createSignal("");
  const activeCurrentSteps = () => arr(t.steps).map(obj).filter((step) => Number(step.stepOrder) === Number(t.currentStep) && step.status === "in_progress");
  const selectedStepId = () => replacementStepId() || str(activeCurrentSteps()[0]?.id);
  return <>
    <header class="page-header"><div><h1>{str(t.ticketNumber) || str(t.title) || "Ticket"}</h1><p>{str(t.title)} · {str(workflow.name)}</p></div><span class={`status status-${t.status}`}>{t.status.replaceAll("_", " ")}</span><WatchToggle ticketId={t.id} /></header>
    <Show when={subStatus()}><p class="sub-status" aria-label="Sub-status"><span class="sub-status-badge">{subStatus()!.name}</span>
      <Show when={subStatusRestrictions(subStatus()).length}><span class="muted"> Not allowed in this sub-status: {subStatusRestrictions(subStatus()).join(", ")}.</span></Show></p></Show>
    <Show when={props.error}><p role="alert">{props.error}</p></Show>
    <dl class="ticket-metadata"><dt>Author</dt><dd>{userLabel(author) || "Unknown"}</dd><dt>Created</dt><dd>{formatJst(str(t.createdAt) || null)}</dd><dt>Updated</dt><dd>{formatJst(str(t.updatedAt) || null)}</dd><dt>Opened</dt><dd>{formatJst(str(t.openedAt) || null)}</dd><dt>Current step</dt><dd>{String(t.currentStep ?? "—")}</dd></dl>
    <Show when={currentSession}><ActionBar ticket={t} me={{ id: currentSession!.user.id, permissions: currentSession!.permissions }} busy={props.busy} onAction={props.onAction} /></Show>
    <Show when={currentSession}><ControlsBar controls={props.controls} steps={(t.steps ?? []) as { stepOrder?: unknown; title?: unknown }[]} busy={props.busy} commentingEnabled={workflow.commentingEnabled !== false} onCancelApproval={props.onCancelApproval} onRaise={props.onRaise} onAddStep={props.onAddStep} /></Show>
    <Show when={currentSession?.permissions.includes("ticket_write") && t.status === "in_progress"}><form class="action-bar" onSubmit={(event) => { event.preventDefault(); if (selectedStepId() && replacementUserId().trim()) { props.onAssignCurrentApprover(selectedStepId(), replacementUserId().trim()); setReplacementUserId(""); } }}>
      <label>Current step <select value={selectedStepId()} onChange={(event) => setReplacementStepId(event.currentTarget.value)}><For each={activeCurrentSteps()}>{(step) => <option value={str(step.id)}>{str(step.title) || str(step.id)}</option>}</For></select></label>
      <label>Add approver <input value={replacementUserId()} onInput={(event) => setReplacementUserId(event.currentTarget.value)} placeholder="Active user ID" /></label>
      <button type="submit" disabled={props.busy || !replacementUserId().trim()}>Add approver</button>
    </form></Show>
    <TicketLabels ticketId={t.id} labels={labels()} onChanged={props.onChanged} />
    <section><h2>Form inputs</h2><dl class="ticket-inputs"><For each={arr(t.inputs)}>{(input) => <><dt>{inputLabel(input)}</dt><dd>{inputValueText(input)}<Show when={attachmentsOf(input).length}><ul><For each={attachmentsOf(input)}>{(file) => <li><a href={file.url}>{file.filename}</a></li>}</For></ul></Show></dd></>}</For></dl>
      <For each={arr(t.slipItems)}>{(slip, index) => <section><h3>{str(obj(slip).title) || `Slip row ${index() + 1}`}</h3><dl><For each={arr(obj(slip).inputs)}>{(input) => <><dt>{inputLabel(input)}</dt><dd>{inputValueText(input)}<Show when={attachmentsOf(input).length}><ul><For each={attachmentsOf(input)}>{(file) => <li><a href={file.url}>{file.filename}</a></li>}</For></ul></Show></dd></>}</For></dl></section>}</For>
    </section>
    <section><h2>Approval steps</h2><StepList steps={t.steps ?? []} currentStep={Number(t.currentStep ?? 0)} removableStepIds={props.controls.removableCustomStepIds} busy={props.busy} onRemoveStep={props.onRemoveStep} /></section>
    <Show when={currentSession && t.status === "in_progress"}><section class="action-bar" aria-label="Approval hold"><h2>Approval hold</h2><For each={arr(t.steps).flatMap((step) => {
      const row = obj(step);
      if (Number(row.stepOrder) !== Number(t.currentStep) || row.status !== "in_progress") return [];
      return arr(row.assignees).map(obj).filter((assignee) => assignee.completed !== true && (obj(assignee.user).id === currentSession!.user.id || arr(assignee.proxyUserIds).includes(currentSession!.user.id)));
    })}>{(assignee) => <button type="button" disabled={props.busy} aria-pressed={assignee.pending === true} onClick={() => props.onSetHold(str(assignee.id), assignee.pending !== true)}>{assignee.pending === true ? "Release approval hold" : "Hold approval reminders"}</button>}</For></section></Show>
    <Show when={props.links.length}><section><h2>Related tickets</h2><ul><For each={props.links}>{(link) => <li><A href={`/tickets/${link.id}`}>{link.ticketNumber || link.title || link.id}</A></li>}</For></ul></section></Show>
    <Comments ticketId={t.id} enabled={workflow.commentingEnabled !== false} onChanged={props.onChanged} />
    <Viewers ticketId={t.id} onChanged={props.onChanged} />
  </>;
}

export default function TicketDetail() {
  const params = useParams(); const [ticket, { refetch }] = createResource(() => params.id, (id) => apiFetch<TicketDetailType>("GET", `/v1/tickets/${id}`));
  const [links] = createResource(() => params.id, (id) => apiFetch<TicketWithStep[]>("GET", `/v1/tickets/${id}/links`));
  const [controls, { refetch: refetchControls }] = createResource(() => params.id, (id) => apiFetch<Partial<TicketControls>>("GET", uiTicketPath(id, "/controls")).then(normalizeControls).catch(() => NO_CONTROLS));
  const [busy, setBusy] = createSignal(false); const [error, setError] = createSignal("");
  const refresh = async () => { await Promise.all([refetch(), refetchControls()]); };
  const control = async (request: () => Promise<unknown>, failure: string) => {
    setBusy(true); setError("");
    try { await request(); await refresh(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : failure); }
    finally { setBusy(false); }
  };
  const cancelApproval = (comment?: string) => void control(() => apiFetch("POST", uiTicketPath(params.id, "/cancel-approval"), comment ? { comment } : undefined), "Unable to cancel the approval");
  const raise = (to?: number) => void control(() => apiFetch("POST", uiTicketPath(params.id, "/raise"), to === undefined ? undefined : { to }), "Unable to raise the ticket");
  const addStep = (body: CustomStepBody) => void control(() => apiFetch("POST", uiTicketPath(params.id, "/custom-steps"), body), "Unable to add the step");
  const removeStep = (stepId: string) => void control(() => apiFetch("DELETE", uiTicketPath(params.id, `/custom-steps/${encodeURIComponent(stepId)}`)), "Unable to remove the step");
  const assignCurrentApprover = (ticketStepId: string, userId: string) => void control(() => apiFetch("POST", uiTicketPath(params.id, "/assignees"), { ticketStepId, userId }), "Unable to add the approver");
  const setHold = (assigneeId: string, pending: boolean) => void control(() => apiFetch("POST", uiTicketPath(params.id, `/assignees/${encodeURIComponent(assigneeId)}/pending`), { pending }), "Unable to update the approval hold");
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
      <TicketContent ticket={detail()!} links={links() ?? []} busy={busy()} error={error()} controls={controls() ?? NO_CONTROLS} onCancelApproval={cancelApproval} onRaise={raise} onAddStep={addStep} onRemoveStep={removeStep} onAssignCurrentApprover={assignCurrentApprover} onSetHold={setHold} onAction={action} onChanged={refresh} />
    </Show></Show>
  </main>;
}
