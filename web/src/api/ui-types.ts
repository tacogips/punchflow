// Types of the punchflow-only /ui/api/ endpoints (watch subscriptions and the
// local notification inbox). They are not part of kickflow's /v1 API.

export type WatchSource = "" | "manual" | "workflow" | "applicant" | "approver";

export interface WatchState {
  ticketId: string;
  watching: boolean;
  source: WatchSource;
  /** True when the workflow definition lists the user, even after an opt-out. */
  workflowWatcher: boolean;
  /** True when the user would watch automatically (applicant, approver, workflow) but opted out. */
  optedOut: boolean;
}

export type NotificationKind =
  | "ticket_applied" | "approval_requested" | "ticket_approved" | "ticket_confirmed"
  | "ticket_rejected" | "ticket_denied" | "ticket_completed" | "ticket_withdrawn"
  | "ticket_archived" | "ticket_updated" | "comment_created" | "comment_mention";

export interface AppNotification {
  id: string;
  kind: NotificationKind | string;
  ticketId: string;
  ticketNumber: string | null;
  ticketTitle: string | null;
  /** False when the user can no longer view the ticket; number and title are then withheld. */
  ticketVisible: boolean;
  commentId: string | null;
  actor: { id: string; name: string } | null;
  excerpt: string;
  read: boolean;
  readAt: string | null;
  createdAt: string;
}
