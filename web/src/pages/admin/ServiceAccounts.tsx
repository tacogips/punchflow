import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { ServiceAccount } from "../../api/types";
import OneTimeSecret from "../../components/integration/OneTimeSecret";

export default function ServiceAccounts() {
  const [accounts, { refetch }] = createResource(() => apiList<ServiceAccount>("/admin/api/service-accounts"));
  const [name, setName] = createSignal(""); const [description, setDescription] = createSignal("");
  const [secret, setSecret] = createSignal(""); const [error, setError] = createSignal("");
  const create = async (event: SubmitEvent) => {
    event.preventDefault(); setError("");
    try {
      const result = await apiFetch<{ token: string }>("POST", "/admin/api/service-accounts", { name: name().trim(), description: description().trim() });
      setSecret(result.token); setName(""); setDescription(""); await refetch();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not create service account."); }
  };
  const revoke = async (id: string) => {
    if (!confirm("Revoke this service account? Its integrations will stop working.")) return;
    try { await apiFetch("DELETE", `/admin/api/service-accounts/${encodeURIComponent(id)}`); await refetch(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not revoke service account."); }
  };
  return <main class="page"><h1>Service Accounts</h1>
    <p>Service accounts use tenant-wide tokens. Send <code>X-Caller-Id</code> with the user UUID to impersonate a user for an API request.</p>
    <form class="inline-form" onSubmit={create}><label>Name <input required value={name()} onInput={(event) => setName(event.currentTarget.value)} /></label><label>Description <input value={description()} onInput={(event) => setDescription(event.currentTarget.value)} /></label><button>Create service account</button></form>
    <Show when={secret()}><OneTimeSecret value={secret()} onClose={() => setSecret("")} /></Show>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show>
    <Show when={!accounts.loading} fallback={<p>Loading service accounts…</p>}><div class="table-wrap"><table><thead><tr><th>Name</th><th>Description</th><th></th></tr></thead><tbody>
      <For each={accounts()?.items ?? []}>{(account) => <tr><td>{account.name}</td><td>{account.description}</td><td><button class="danger" onClick={() => revoke(account.id)}>Revoke</button></td></tr>}</For>
    </tbody></table><Show when={(accounts()?.items.length ?? 0) === 0}><p class="muted">No service accounts yet.</p></Show></div></Show>
  </main>;
}
