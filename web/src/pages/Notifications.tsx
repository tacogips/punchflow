import { For, Show, createResource, createSignal } from "solid-js";
import { A } from "@solidjs/router";
import { apiFetch, apiList } from "../api/client";
import type { AppNotification } from "../api/ui-types";
import { formatJst } from "../lib/format";
import { notificationHref, notificationSummary, ticketLabel } from "../lib/notification-view";
import { refreshUnread, setUnreadCount } from "../lib/unread";
import "../components/notifications/notifications.css";

export default function Notifications() {
  const [unreadOnly, setUnreadOnly] = createSignal(false);
  const [page, setPage] = createSignal(1);
  const [error, setError] = createSignal("");
  const [result, { refetch }] = createResource(
    () => ({ unread: unreadOnly(), page: page() }),
    ({ unread, page: current }) => apiList<AppNotification>("/ui/api/notifications", { page: String(current), perPage: "25", ...(unread ? { unread: "true" } : {}) }),
  );
  const run = async (action: () => Promise<unknown>) => {
    setError("");
    try { await action(); await refetch(); await refreshUnread(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Unable to update notifications"); }
  };
  const markRead = (id: string) => run(() => apiFetch<{ count: number }>("POST", `/ui/api/notifications/${id}/read`).then((r) => setUnreadCount(r.count)));
  const markAll = () => run(() => apiFetch<{ count: number }>("POST", "/ui/api/notifications/read-all").then((r) => setUnreadCount(r.count)));
  const move = (rel: "next" | "prev") => {
    const href = result()?.pagination.links[rel]; if (!href) return;
    const next = Number(new URL(href, window.location.origin).searchParams.get("page")); if (next > 0) setPage(next);
  };
  return <main class="page"><header class="page-header"><h1>Notifications</h1>
    <button type="button" onClick={markAll}>Mark all as read</button></header>
    <label class="checkbox-option"><input type="checkbox" checked={unreadOnly()} onChange={(event) => { setPage(1); setUnreadOnly(event.currentTarget.checked); }} />Unread only</label>
    <Show when={error()}><p role="alert">{error()}</p></Show>
    <Show when={!result.loading} fallback={<p>Loading notifications…</p>}><Show when={!result.error} fallback={<p role="alert">Unable to load notifications.</p>}>
      <p>{result()?.pagination.total ?? 0} notifications</p>
      <Show when={(result()?.items.length ?? 0) > 0} fallback={<p class="empty-state">No notifications.</p>}>
        <ul class="notification-list"><For each={result()?.items ?? []}>{(item) => <li classList={{ unread: !item.read, read: item.read }}>
          <strong>{notificationSummary(item)}</strong>
          <Show when={notificationHref(item)} fallback={<span>{ticketLabel(item)}</span>}>{(href) => <A href={href()} onClick={() => { if (!item.read) void markRead(item.id); }}>{ticketLabel(item)}</A>}</Show>
          <Show when={item.excerpt}><p class="notification-excerpt">{item.excerpt}</p></Show>
          <span class="notification-meta"><time>{formatJst(item.createdAt)}</time>
            <Show when={!item.read}><button type="button" onClick={() => void markRead(item.id)}>Mark as read</button></Show></span>
        </li>}</For></ul>
      </Show>
      <nav class="pagination" aria-label="Notification pages"><button disabled={!result()?.pagination.links.prev} onClick={() => move("prev")}>Previous</button><span>Page {result()?.pagination.page}</span><button disabled={!result()?.pagination.links.next} onClick={() => move("next")}>Next</button></nav>
    </Show></Show>
  </main>;
}
