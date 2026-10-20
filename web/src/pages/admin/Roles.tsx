import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { Role, User } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
function permissionName(value: unknown): string {
  if (typeof value === "string") return value;
  if (value && typeof value === "object" && "permission" in value) return String(value.permission);
  return String(value ?? "");
}
export default function Roles() {
  const [selectedId, setSelectedId] = createSignal("");
  const [roles] = createResource(async () => { const data = await apiList<Role>("/v1/roles"); if (!selectedId()) setSelectedId(data.items[0]?.id ?? ""); return data.items; });
  const [detail] = createResource(selectedId, (id) => id ? apiFetch<Role>("GET", `/v1/roles/${encodeURIComponent(id)}`) : undefined);
  const [members] = createResource(selectedId, (id) => id ? apiList<User>(`/v1/roles/${encodeURIComponent(id)}/memberships`) : undefined);
  return <main class="page"><h1>Roles</h1><Show when={roles.error}><ReadError error={roles.error} /></Show><Show when={roles()}>{(items) => <label>Role <select value={selectedId()} onChange={(event) => setSelectedId(event.currentTarget.value)}><For each={items()}>{(role) => <option value={role.id}>{role.name}</option>}</For></select></label>}</Show>
    <Show when={detail.error}><ReadError error={detail.error} /></Show><Show when={detail()}>{(role) => <><h2>{role().name}</h2><h3>Permissions</h3><ul><For each={(role().permissionList as unknown[] | undefined) ?? []}>{(permission) => <li>{permissionName(permission)}</li>}</For></ul>
      <h3>Members</h3><Show when={members.error}><ReadError error={members.error} /></Show><Show when={members()}>{(data) => <PagedTable items={data().items} columns={[{ key: "name", label: "Name" }, { key: "email", label: "Email" }, { key: "status", label: "Status" }]} />}</Show></>}</Show>
  </main>;
}
