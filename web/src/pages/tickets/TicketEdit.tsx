import { For, Show, createEffect, createResource, createSignal } from "solid-js";
import { A, useParams, useSearchParams } from "@solidjs/router";
import type { FormField, SectionListItem, TicketDetail, Team, Workflow, WorkflowDetail } from "../../api/types";
import { apiFetch, apiList } from "../../api/client";
import { toTicketInputs, type FieldState } from "../../lib/form-model";
import { approverEditable, buildFormPayload, conditionValues, formStateFromTicket, sectionConditionMet, sectionViewable, slipKey, slipRowsFromTicket } from "../../lib/ticket-form";
import { buildApprovers } from "../../lib/ticket-view";
import type { SlipRows } from "../../components/tickets/SlipTable";
import FormFields from "../../components/tickets/FormFields";
import SlipTable from "../../components/tickets/SlipTable";
import { session } from "../../session";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";
const sections = (workflow: WorkflowDetail): SectionListItem[] => workflow.sectionList ?? [];
export default function TicketEdit() {
  const params = useParams(); const [search] = useSearchParams(); const [chosenWorkflow, setChosenWorkflow] = createSignal(str(search.workflow ?? ""));
  const [ticket, { refetch: refetchTicket }] = createResource(() => params.id, (id) => id ? apiFetch<TicketDetail>("GET", `/v1/tickets/${id}`) : Promise.resolve(undefined));
  const [workflows] = createResource(() => apiList<Workflow>("/v1/workflows", { perPage: "100" }).then((result) => result.items));
  const workflowId = () => chosenWorkflow() || str(obj(obj(ticket()).workflow).id);
  const [workflow] = createResource(workflowId, (id) => id ? apiFetch<WorkflowDetail>("GET", `/v1/workflows/${id}`) : Promise.resolve(undefined));
  const me = () => session()?.user;
  const [teams] = createResource(() => {
    const clientID = proxyClientUserId();
    return [me()?.id ?? "", clientID, workflowId()].join("|");
  }, async (key) => {
    const [userID, clientID, selectedWorkflowID] = key.split("|");
    if (!userID) return [];
    const query = new URLSearchParams();
    if (clientID) query.set("proxyClientUserId", clientID);
    if (clientID) query.set("workflowId", selectedWorkflowID);
    return apiFetch<Team[]>("GET", `/ui/api/application-teams${query.size ? `?${query}` : ""}`);
  });
  const [form, setForm] = createSignal<Record<string, FieldState>>({}); const [slips, setSlips] = createSignal<SlipRows>({});
  const [title, setTitle] = createSignal(""); const [teamId, setTeamId] = createSignal(""); const [proxyClientUserId, setProxyClientUserId] = createSignal("");
  const [approvers, setApprovers] = createSignal<Record<string, string>>({}); const [submissionComment, setSubmissionComment] = createSignal(""); const [errors, setErrors] = createSignal<Record<string, string[]>>({}); const [error, setError] = createSignal(""); const [busy, setBusy] = createSignal(false); const [initialized, setInitialized] = createSignal("");
  createEffect(() => {
    const definition = workflow(); const existing = ticket(); const key = `${workflowId()}:${existing?.id ?? "new"}`;
    if (!definition || initialized() === key) return;
    setForm(formStateFromTicket(definition, existing)); setSlips(slipRowsFromTicket(definition, existing)); setTitle(str(existing?.title)); setProxyClientUserId(str(obj(obj(existing).proxyClientUser).id));
    const myTeams = teams() ?? []; setTeamId(str(obj(obj(existing).authorTeam).id) || (!params.id ? str(myTeams[0]?.id) : "")); setInitialized(key);
  });
  createEffect(() => {
    const myTeams = teams() ?? [];
    if (!params.id && !teamId() && myTeams.length > 0) setTeamId(str(myTeams[0]?.id));
  });
  const approverEdit = () => params.id !== undefined && ticket()?.status === "in_progress";
  const routeSteps = () => arr(obj(obj(workflow()).route).steps);
  const memberTeams = (): Team[] => teams.state === "ready" ? teams() ?? [] : [];
  const canView = (section: SectionListItem) => sectionViewable(section, ticket(), { userId: me()?.id, permissions: session()?.permissions, teamIds: memberTeams().map((team) => team.id) });
  const conditionInputs = () => { const definition = workflow(); return definition ? conditionValues(toTicketInputs(definition, form())) : {}; };
  const editableField = (field: FormField) => !approverEdit() || approverEditable(workflow() as WorkflowDetail, field);
  const save = async (status: "draft" | "in_progress") => {
    setBusy(true); setError(""); setErrors({});
    const definition = workflow(); if (!definition) { setError("Choose a workflow first."); setBusy(false); return; }
    const formPayload: Record<string, unknown> = { ...buildFormPayload(definition, form(), slips(), { viewable: canView, approver: approverEdit() }) };
    const approversPayload = buildApprovers(approvers());
    const payload: Record<string, unknown> = params.id && approverEdit() ? formPayload : {
      ...(status === "draft" ? { status: ticket()?.status === "rejected" ? "rejected" : "draft" } : params.id ? {} : { status: "draft" }),
      workflowId: workflowId(), authorTeamId: teamId(), proxyClientUserId: proxyClientUserId() || null,
      title: title().trim() || null, ...formPayload,
      ...(params.id ? (approversPayload.length ? { approvers: approversPayload } : {}) : { approvers: approversPayload }),
    };
    try {
      const saved = params.id
        ? await apiFetch<TicketDetail>("PATCH", `/v1/tickets/${params.id}`, payload)
        : await apiFetch<TicketDetail>("POST", "/v1/tickets", payload);
      if (status === "in_progress" && !approverEdit()) {
        await apiFetch("POST", `/ui/api/tickets/${saved.id}/submit`, submissionComment().trim() ? { comment: submissionComment().trim() } : undefined);
      }
      if (status === "draft") window.location.assign(`/tickets/${saved.id}`);
      else window.location.assign(`/tickets/${saved.id}`);
    } catch (cause) {
      const problem = cause as Error & { errors?: Record<string, string[]> };
      setError(problem.message || "Unable to save ticket"); setErrors(problem.errors ?? { base: [problem.message || "Unable to save ticket"] });
      await refetchTicket();
    } finally { setBusy(false); }
  };
  const loading = () => ticket.loading || workflows.loading || Boolean(workflowId()) && workflow.loading;
  return <main class="page"><A href={params.id ? `/tickets/${params.id}` : "/tickets"}>Cancel</A><h1>{params.id ? "Edit ticket" : "New ticket"}</h1>
    <Show when={error()}><p role="alert">{error()}</p></Show><Show when={errors().base}><ul role="alert"><For each={errors().base}>{(message) => <li>{message}</li>}</For></ul></Show>
    <Show when={!loading()} fallback={<p>Loading ticket form…</p>}>
      <Show when={params.id || chosenWorkflow()} fallback={<label>Workflow<select value={chosenWorkflow()} onChange={(event) => { setChosenWorkflow(event.currentTarget.value); setInitialized(""); }}><option value="">Choose workflow…</option><For each={workflows() ?? []}>{(item) => <option value={item.id}>{item.name}</option>}</For></select></label>}>
        <Show when={workflow()} fallback={<p>Select a workflow to start.</p>}>{(definition) => <form onSubmit={(event) => { event.preventDefault(); void save("in_progress"); }}>
          <Show when={!approverEdit()}><label>Application team<select value={teamId()} onChange={(event) => setTeamId(event.currentTarget.value)}><option value="">Choose team…</option><For each={teams() ?? []}>{(team) => <option value={team.id}>{team.fullName || team.name}</option>}</For></select></label>
            <label>Proxy client user ID (optional)<input value={proxyClientUserId()} onInput={(event) => setProxyClientUserId(event.currentTarget.value)} /></label>
            <Show when={obj(definition()).titleInputMode === "input" || obj(definition()).allowTitle === true}><label>Title<input value={title()} onInput={(event) => setTitle(event.currentTarget.value)} /></label></Show>
          </Show>
          <For each={sections(definition())}>{(section) => <Show when={canView(section) && sectionConditionMet(section, conditionInputs())}><section><h2>{section.title || (section.sectionType === "slip" ? "Items" : "Form")}</h2>
            <Show when={section.sectionType === "form"} fallback={<SlipTable workflowId={workflowId()} code={slipKey(section)} title={section.title ?? "Items"} fields={(section.slipFields ?? []).filter(editableField)} rows={slips} setRows={setSlips} errors={errors()} />}>
              <FormFields workflowId={workflowId()} fields={(section.formFields ?? []).filter(editableField)} state={form} setState={setForm} errors={errors()} />
            </Show>
          </section></Show>}</For>
          <For each={routeSteps().filter((step) => obj(step).stepType === "author_customizable")}>{(step) => { const code = str(obj(step).code); return <label>Approvers for {str(obj(step).title) || code}<input value={approvers()[code] ?? ""} onInput={(event) => setApprovers((current) => ({ ...current, [code]: event.currentTarget.value }))} placeholder="User IDs separated by commas" /></label>; }}</For>
          <Show when={!approverEdit() && obj(workflow()).commentingEnabled !== false}><label>Submission comment<textarea value={submissionComment()} onInput={(event) => setSubmissionComment(event.currentTarget.value)} /></label></Show>
          <Show when={errors().base?.length}><ul class="field-errors"><For each={errors().base}>{(message) => <li>{message}</li>}</For></ul></Show>
          <div class="form-actions"><Show when={!approverEdit()}><button type="button" disabled={busy()} onClick={() => void save("draft")}>Save draft</button></Show><button disabled={busy()}>{approverEdit() ? "Save changes" : params.id ? "Save and submit" : "Submit"}</button></div>
        </form>}</Show>
      </Show>
    </Show>
  </main>;
}
