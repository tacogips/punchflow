import { Show, onCleanup, onMount } from "solid-js";
import { badgeText } from "../../lib/notification-view";
import { refreshUnread, unreadCount } from "../../lib/unread";
import "./notifications.css";

const POLL_MS = 30_000;

/** Unread notification count badge for the sidebar link; refreshes periodically. */
export default function UnreadBadge() {
  onMount(() => {
    void refreshUnread();
    const timer = window.setInterval(() => void refreshUnread(), POLL_MS);
    onCleanup(() => window.clearInterval(timer));
  });
  return <Show when={badgeText(unreadCount())}>{(text) =>
    <span class="unread-badge" role="status" aria-label={`${unreadCount()} unread notifications`}>{text()}</span>}</Show>;
}
