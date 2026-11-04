import { createSignal } from "solid-js";
import { apiFetch } from "../api/client";

/** Unread notification count shown in the sidebar and kept in sync by the inbox page. */
export const [unreadCount, setUnreadCount] = createSignal(0);

/** Reloads the unread count; failures keep the previous value. */
export async function refreshUnread(): Promise<void> {
  try {
    const result = await apiFetch<{ count: number }>("GET", "/ui/api/notifications/unread-count");
    setUnreadCount(Number(result?.count) || 0);
  } catch {
    // The badge is informational; ignore transient errors.
  }
}
