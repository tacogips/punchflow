export const WEBHOOK_EVENT_TYPES = [
  "ticket_created", "ticket_updated", "ticket_opened", "ticket_approved", "ticket_confirmed",
  "ticket_rejected", "ticket_denied", "ticket_completed", "ticket_withdrawn", "ticket_archived",
  "comment_created", "comment_updated",
] as const;

export function deliveryStatusLabel(ev: { status: string; attemptCount: number; nextAttemptAt: string | null }): string {
  if (ev.status === "succeeded") return "delivered";
  if (ev.status === "failed") return "failed";
  if (ev.status === "pending" && ev.attemptCount > 0) return "retrying";
  if (ev.status === "pending") return "pending";
  return ev.status;
}

export function validateWebhookForm(f: { name: string; url: string; eventTypes: string[] }): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!f.name.trim()) errors.name = "Name is required.";
  if (!/^https?:\/\//i.test(f.url.trim())) errors.url = "URL must start with http:// or https://.";
  return errors;
}
