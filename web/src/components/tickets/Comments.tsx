import { For, Show, createResource, createSignal } from "solid-js";
import type { Comment } from "../../api/types";
import { apiFetch, apiList, apiUpload } from "../../api/client";
import { attachmentsOf, userLabel } from "../../lib/ticket-view";
import { session } from "../../session";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";
export default function Comments(props: { ticketId: string; enabled: boolean; onChanged: () => void }) {
  const [comments, { refetch }] = createResource(() => props.ticketId, (id) => apiList<Comment>(`/v1/tickets/${id}/comments`, { perPage: "100" }).then((result) => result.items));
  const [body, setBody] = createSignal(""); const [files, setFiles] = createSignal<string[]>([]); const [error, setError] = createSignal(""); const [editing, setEditing] = createSignal<string | null>(null); const [busy, setBusy] = createSignal(false);
  const mine = (comment: Comment) => str(obj(comment.author).id) === session()?.user.id || str(obj(comment.user).id) === session()?.user.id;
  const refresh = async () => { await refetch(); props.onChanged(); };
  const addFile = async (file: File) => { const uploaded = await apiUpload("/v1/files", file); setFiles((current) => [...current, uploaded.signedId]); };
  const submit = async (event: SubmitEvent) => {
    event.preventDefault(); setError(""); setBusy(true);
    try { await apiFetch("POST", `/v1/tickets/${props.ticketId}/comments`, { body: body(), files: files() }); setBody(""); setFiles([]); await refresh(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to post comment"); }
    finally { setBusy(false); }
  };
  const save = async (id: string) => { setBusy(true); setError(""); try { await apiFetch("PATCH", `/v1/tickets/${props.ticketId}/comments/${id}`, { body: body() }); setEditing(null); setBody(""); await refresh(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to edit comment"); } finally { setBusy(false); } };
  const remove = async (id: string) => { setBusy(true); setError(""); try { await apiFetch("DELETE", `/v1/tickets/${props.ticketId}/comments/${id}`); await refresh(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to delete comment"); } finally { setBusy(false); } };
  return <section><h2>Comments</h2><Show when={error()}><p role="alert">{error()}</p></Show>
    <For each={comments() ?? []}>{(comment) => <article class="comment"><p>{str(comment.body ?? comment.content)}</p><small>{userLabel(comment.user) || "Unknown"} · {str(comment.createdAt)}</small>
      <Show when={mine(comment) && !comment.deletedAt}><button onClick={() => { setEditing(comment.id); setBody(str(comment.body ?? comment.content)); }}>Edit</button><button disabled={busy()} onClick={() => remove(comment.id)}>Delete</button></Show>
      <Show when={editing() === comment.id}><textarea value={body()} onInput={(event) => setBody(event.currentTarget.value)} /><button disabled={busy()} onClick={() => save(comment.id)}>Save</button><button onClick={() => setEditing(null)}>Cancel</button></Show>
      <Show when={attachmentsOf(comment).length}><ul><For each={attachmentsOf(comment)}>{(file) => <li><a href={file.url}>{file.filename}</a></li>}</For></ul></Show>
    </article>}</For>
    <Show when={props.enabled}><form onSubmit={submit}><label>New comment<textarea value={body()} onInput={(event) => setBody(event.currentTarget.value)} required /></label>
      <label>Attachments<input type="file" multiple onChange={async (event) => { try { for (const file of Array.from(event.currentTarget.files ?? [])) await addFile(file); } catch (cause) { setError(cause instanceof Error ? cause.message : "Upload failed"); } event.currentTarget.value = ""; }} /></label>
      <Show when={files().length}><p>{files().length} attachment(s) ready</p></Show><button disabled={busy()}>Post comment</button>
    </form></Show>
  </section>;
}
