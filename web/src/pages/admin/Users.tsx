import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { User } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Users() {
  const [page, setPage] = createSignal(1);
  const [notice, setNotice] = createSignal("");
  const [actionError, setActionError] = createSignal<unknown>();
  const [result, { refetch }] = createResource(page, (current) => apiList<User>("/v1/users", { page: String(current), perPage: "25" }));
  async function act(user: User, action: "suspend" | "reactivate" | "reinvite") {
    setNotice(""); setActionError(undefined);
    try { await apiFetch("POST", `/v1/users/${encodeURIComponent(user.id)}/${action}`); setNotice(`${user.name}: ${action} completed`); await refetch(); }
    catch (error) { setActionError(error); }
  }
  return <main class="page"><h1>Users</h1><Show when={notice()}><p class="notice">{notice()}</p></Show><Show when={actionError()}><ReadError error={actionError()} /></Show><Show when={result.error}><ReadError error={result.error} /></Show>
    <Show when={result()}>{(data) => <PagedTable items={data().items} columns={[
      { key: "name", label: "Name" }, { key: "email", label: "Email" }, { key: "status", label: "Status" }, { key: "userType", label: "Type" },
    ]} page={data().pagination.page} perPage={data().pagination.perPage} total={data().pagination.total} onPage={setPage} rowKey={(user) => user.id}
    rowActions={(user) => <span class="actions"><Show when={user.status === "activated"}><button onClick={() => void act(user, "suspend")}>Suspend</button></Show><Show when={user.status === "suspended"}><button onClick={() => void act(user, "reactivate")}>Reactivate</button></Show><Show when={user.status === "deactivated"}><button onClick={() => void act(user, "reinvite")}>Reinvite</button></Show></span>} />}</Show>
  </main>;
}
