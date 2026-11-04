import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { User } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
import { session } from "../../session";
export default function Users() {
  const [page, setPage] = createSignal(1);
  const [notice, setNotice] = createSignal("");
  const [actionError, setActionError] = createSignal<unknown>();
  const [fromUserId, setFromUserId] = createSignal("");
  const [toUserId, setToUserId] = createSignal("");
  const [teamId, setTeamId] = createSignal("");
  const [ticketIds, setTicketIds] = createSignal("");
  const [result, { refetch }] = createResource(page, (current) => apiList<User>("/v1/users", { page: String(current), perPage: "25" }));
  async function act(user: User, action: "suspend" | "reactivate" | "reinvite") {
    setNotice(""); setActionError(undefined);
    try { await apiFetch("POST", `/v1/users/${encodeURIComponent(user.id)}/${action}`); setNotice(`${user.name}: ${action} completed`); await refetch(); }
    catch (error) { setActionError(error); }
  }
  async function handover(kind: "applicant-handover" | "approval-handover") {
    setNotice(""); setActionError(undefined);
    try {
      const response = await apiFetch<{ ticketIds: string[] }>("POST", `/ui/api/admin/tickets/${kind}`, {
        fromUserId: fromUserId(), toUserId: toUserId(),
        ...(kind === "applicant-handover" ? { teamId: teamId() } : {}),
        ...(ticketIds().trim() ? { ticketIds: ticketIds().split(",").map((id) => id.trim()).filter(Boolean) } : {}),
      });
      setNotice(`${kind === "applicant-handover" ? "Applicant" : "Approval request"} handover updated ${response.ticketIds.length} ticket(s).`);
    } catch (error) { setActionError(error); }
  }
  return <main class="page"><h1>Users</h1><Show when={notice()}><p class="notice">{notice()}</p></Show><Show when={actionError()}><ReadError error={actionError()} /></Show><Show when={result.error}><ReadError error={result.error} /></Show>
    <Show when={session()?.permissions.includes("ticket_write")}><section class="panel"><h2>User handover</h2><p>Transfer open tickets during a user lifecycle change. Leave ticket IDs blank to transfer all eligible tickets; otherwise enter up to 1,000 comma-separated ticket IDs.</p>
      <label>From user<select value={fromUserId()} onChange={(event) => setFromUserId(event.currentTarget.value)}><option value="">Select user…</option><For each={result()?.items ?? []}>{(user) => <option value={user.id}>{user.name} ({user.email})</option>}</For></select></label>
      <label>To user<select value={toUserId()} onChange={(event) => setToUserId(event.currentTarget.value)}><option value="">Select user…</option><For each={result()?.items ?? []}>{(user) => <option value={user.id}>{user.name} ({user.email})</option>}</For></select></label>
      <label>Target team ID for unproxied applicant tickets<input value={teamId()} onInput={(event) => setTeamId(event.currentTarget.value)} /></label>
      <label>Ticket IDs (optional)<input value={ticketIds()} onInput={(event) => setTicketIds(event.currentTarget.value)} placeholder="ticket-id-1, ticket-id-2" /></label>
      <div class="actions"><button disabled={!fromUserId() || !toUserId()} onClick={() => void handover("applicant-handover")}>Transfer authored tickets</button><button disabled={!fromUserId() || !toUserId()} onClick={() => void handover("approval-handover")}>Transfer approval requests</button></div>
    </section></Show>
    <Show when={result()}>{(data) => <PagedTable items={data().items} columns={[
      { key: "name", label: "Name" }, { key: "email", label: "Email" }, { key: "status", label: "Status" }, { key: "userType", label: "Type" },
    ]} page={data().pagination.page} perPage={data().pagination.perPage} total={data().pagination.total} onPage={setPage} rowKey={(user) => user.id}
    rowActions={(user) => <span class="actions"><Show when={user.status === "activated"}><button onClick={() => void act(user, "suspend")}>Suspend</button></Show><Show when={user.status === "suspended"}><button onClick={() => void act(user, "reactivate")}>Reactivate</button></Show><Show when={user.status === "deactivated"}><button onClick={() => void act(user, "reinvite")}>Reinvite</button></Show></span>} />}</Show>
  </main>;
}
