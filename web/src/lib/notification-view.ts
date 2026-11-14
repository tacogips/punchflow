import type { AppNotification } from "../api/ui-types";

const kindText: Record<string, string> = {
  ticket_applied: "applied for a ticket",
  approval_requested: "needs your approval",
  ticket_approved: "approved a ticket",
  ticket_confirmed: "confirmed a ticket",
  ticket_rejected: "sent a ticket back",
  ticket_denied: "denied a ticket",
  ticket_completed: "completed a ticket",
  ticket_withdrawn: "withdrew a ticket",
  ticket_archived: "archived a ticket",
  ticket_updated: "edited a ticket",
  comment_created: "commented on a ticket",
  comment_mention: "mentioned you in a comment",
};

/** Human-readable label for a notification kind; unknown kinds are shown humanised. */
export function kindLabel(kind: string): string {
  return kindText[kind] ?? kind.replaceAll("_", " ");
}

/** The ticket a notification refers to, by number, then title, then a fallback. */
export function ticketLabel(n: Pick<AppNotification, "ticketVisible" | "ticketNumber" | "ticketTitle">): string {
  if (!n.ticketVisible) return "a ticket you can no longer view";
  const number = n.ticketNumber?.trim();
  const title = n.ticketTitle?.trim();
  if (number && title) return `${number} ${title}`;
  return number || title || "Untitled ticket";
}

/** One-line summary such as "Sales User mentioned you in a comment". */
export function notificationSummary(n: AppNotification): string {
  const who = n.actor?.name?.trim();
  const what = kindLabel(n.kind);
  // Approval requests read as a task, not as an action of the requester.
  if (n.kind === "approval_requested") return who ? `${who} requested approval: ${what}` : `Approval needed: ${what}`;
  return who ? `${who} ${what}` : `Someone ${what}`;
}

/** Link target of a notification, or "" when the ticket is no longer viewable. */
export function notificationHref(n: Pick<AppNotification, "ticketVisible" | "ticketId">): string {
  return n.ticketVisible ? `/tickets/${n.ticketId}` : "";
}

/** Sidebar badge text: empty for none, capped at 99+. */
export function badgeText(count: number): string {
  if (!Number.isFinite(count) || count <= 0) return "";
  return count > 99 ? "99+" : String(Math.trunc(count));
}
