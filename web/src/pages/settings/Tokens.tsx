import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { ApiToken } from "../../api/types";
import OneTimeSecret from "../../components/integration/OneTimeSecret";

export default function Tokens() {
  const [tokens, { refetch }] = createResource(() => apiList<ApiToken>("/admin/api/tokens"));
  const [name, setName] = createSignal("");
  const [secret, setSecret] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const create = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setBusy(true);
    try {
      const result = await apiFetch<{ token: string }>("POST", "/admin/api/tokens", { name: name().trim() });
      setSecret(result.token); setName(""); await refetch();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not create token."); }
    finally { setBusy(false); }
  };
  const revoke = async (id: string) => {
    if (!confirm("Revoke this API token? Existing clients will stop working.")) return;
    try { await apiFetch("DELETE", `/admin/api/tokens/${encodeURIComponent(id)}`); await refetch(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Could not revoke token."); }
  };
  return <main class="page"><h1>API Tokens</h1>
    <p>Personal access tokens act as your user. The token value is shown once when created.</p>
    <form class="inline-form" onSubmit={create}><label>Name <input required value={name()} onInput={(event) => setName(event.currentTarget.value)} /></label><button disabled={busy()}>{busy() ? "Creating…" : "Create token"}</button></form>
    <Show when={secret()}><OneTimeSecret value={secret()} onClose={() => setSecret("")} /></Show>
    <Show when={error()}><p class="error" role="alert">{error()}</p></Show>
    <Show when={!tokens.loading} fallback={<p>Loading tokens…</p>}><div class="table-wrap"><table><thead><tr><th>Name</th><th>Created</th><th>Last used</th><th></th></tr></thead><tbody>
      <For each={tokens()?.items ?? []}>{(token) => <tr><td>{token.name}</td><td>{token.createdAt}</td><td>{token.lastUsedAt ?? "Never"}</td><td><button class="danger" onClick={() => revoke(token.id)}>Revoke</button></td></tr>}</For>
    </tbody></table><Show when={(tokens()?.items.length ?? 0) === 0}><p class="muted">No API tokens yet.</p></Show></div></Show>
  </main>;
}
