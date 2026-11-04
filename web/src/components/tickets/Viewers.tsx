import { For, Show, createResource, createSignal } from "solid-js";
import type { Grade, TicketViewer } from "../../api/types";
import { apiFetch, apiList } from "../../api/client";
import { userLabel } from "../../lib/ticket-view";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const str = (v: unknown): string => typeof v === "string" ? v : "";
export default function Viewers(props: { ticketId: string; onChanged: () => void }) {
  const [viewers, { refetch }] = createResource(() => props.ticketId, (id) => apiList<TicketViewer>(`/v1/tickets/${id}/viewers`, { perPage: "100" }).then((result) => result.items));
  const [grades] = createResource(() => apiList<Grade>("/v1/grades", { perPage: "100" }).then((result) => result.items));
  const [kind, setKind] = createSignal("user"); const [id, setId] = createSignal(""); const [gradeId, setGradeId] = createSignal(""); const [descendants, setDescendants] = createSignal(false); const [error, setError] = createSignal(""); const [busy, setBusy] = createSignal(false);
  const label = (viewer: TicketViewer) => {
    const userName = userLabel(viewer.user);
    if (userName) return userName;
    const team = obj(viewer.team);
    const teamName = str(team.fullName) || str(team.name);
    if (teamName) {
      const gradeName = str(obj(viewer.grade).name);
      return gradeName ? `${teamName} · ${gradeName}` : teamName;
    }
    return str(viewer.userId ?? viewer.teamId ?? viewer.id);
  };
  const refresh = async () => { await refetch(); props.onChanged(); };
  const add = async (event: SubmitEvent) => { event.preventDefault(); setError(""); setBusy(true); try {
    await apiFetch("POST", `/v1/tickets/${props.ticketId}/viewers`, kind() === "user" ? { userId: id() } : { teamId: id(), gradeId: gradeId() || null, descendants: descendants() }); setId(""); await refresh();
  } catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to add viewer"); } finally { setBusy(false); } };
  const remove = async (viewer: TicketViewer) => { const viewerId = str(viewer.userId ?? viewer.teamId ?? viewer.id); setError(""); setBusy(true); try { await apiFetch("DELETE", `/v1/tickets/${props.ticketId}/viewers/${viewerId}`); await refresh(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to remove viewer"); } finally { setBusy(false); } };
  return <section><h2>Viewers</h2><Show when={error()}><p role="alert">{error()}</p></Show>
    <ul><For each={viewers() ?? []}>{(viewer) => <li>{label(viewer)} <button disabled={busy()} onClick={() => remove(viewer)}>Remove</button></li>}</For></ul>
    <form onSubmit={add}><label>Viewer type<select value={kind()} onChange={(event) => setKind(event.currentTarget.value)}><option value="user">User</option><option value="team">Team</option></select></label>
      <label>{kind() === "user" ? "User ID" : "Team ID"}<input value={id()} onInput={(event) => setId(event.currentTarget.value)} required /></label>
      <Show when={kind() === "team"}><label>Grade<select value={gradeId()} onChange={(event) => setGradeId(event.currentTarget.value)}><option value="">Any grade</option><For each={grades() ?? []}>{(grade) => <option value={grade.id}>{grade.name}</option>}</For></select></label><label><input type="checkbox" checked={descendants()} onChange={(event) => setDescendants(event.currentTarget.checked)} /> Include descendants</label></Show>
      <button disabled={busy()}>Add viewer</button>
    </form>
  </section>;
}
