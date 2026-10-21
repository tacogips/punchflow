import type { TicketDetail, TicketStep } from "../api/types";

export type TicketAction = "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit";

export function buildApprovers(raw: Record<string, string>): { routeStepCode: string; userId: string[] }[] {
  return Object.entries(raw).flatMap(([routeStepCode, value]) => {
    const userId = [...new Set(value.split(/[\s,]+/).map((id) => id.trim()).filter(Boolean))];
    return userId.length ? [{ routeStepCode, userId }] : [];
  });
}

type Loose = Record<string, unknown>;
const asRecord = (value: unknown): Loose => value && typeof value === "object" ? value as Loose : {};
const asArray = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const asString = (value: unknown): string => typeof value === "string" ? value : "";

export function userLabel(value: unknown): string {
  const user = asRecord(value);
  const fullName = asString(user.fullName).trim();
  if (fullName) return fullName;
  const firstName = asString(user.firstName).trim();
  const lastName = asString(user.lastName).trim();
  const personalName = [lastName, firstName].filter(Boolean).join(" ");
  return personalName || asString(user.email) || asString(user.id);
}

export function attachmentsOf(value: unknown): { filename: string; url: string }[] {
  return asArray(asRecord(value).attachments).flatMap((entry) => {
    const attachment = asRecord(entry);
    const filename = asString(attachment.filename);
    const url = asString(attachment.url);
    return filename && url ? [{ filename, url }] : [];
  });
}

export function inputLabel(value: unknown): string {
  const input = asRecord(value);
  const field = asRecord(input.formField ?? input.slipField);
  return asString(field.title) || asString(field.code)
    || asString(input.formFieldId) || asString(input.slipFieldId);
}

export function inputValueText(value: unknown): string {
  const input = asRecord(value);
  const resolved: [unknown, (item: unknown) => string][] = [
    [input.generalMasterItems, (item) => asString(asRecord(item).name)],
    [input.users, userLabel],
    [input.teams, (item) => asString(asRecord(item).fullName) || asString(asRecord(item).name)],
    [input.inputTickets, (item) => asString(asRecord(item).title) || asString(asRecord(item).ticketNumber)],
  ];
  for (const [items, label] of resolved) {
    if (Array.isArray(items) && items.length) {
      return items.map(label).filter(Boolean).join(", ");
    }
  }
  const fallback = input.value;
  if (Array.isArray(fallback)) {
    return fallback.map((item) => item == null ? "" : typeof item === "object" ? JSON.stringify(item) : String(item))
      .filter(Boolean).join(", ");
  }
  if (fallback == null) return "—";
  return typeof fallback === "object" ? JSON.stringify(fallback) : String(fallback);
}

function assigneeCanAct(step: TicketStep, userId: string): boolean {
  return asArray(step.assignees).some((value) => {
    const assignee = asRecord(value);
    if (assignee.completed === true) return false;
    const user = asRecord(assignee.user);
    const proxies = asArray(assignee.proxyUserIds).map(asString);
    return user.id === userId || proxies.includes(userId);
  });
}

function currentStep(ticket: TicketDetail): TicketStep | undefined {
  const current = Number(asRecord(ticket).currentStep ?? 0);
  return ticket.steps.find((step) => asArray(step.assignees).some((value) => asRecord(value).current === true))
    ?? ticket.steps.find((step) => Number(asRecord(step).stepOrder) === current);
}

export function availableActions(t: TicketDetail, me: { id: string; permissions: string[] }): TicketAction[] {
  const actions: TicketAction[] = [];
  const ticket = asRecord(t);
  const author = asRecord(ticket.author);
  const isAuthor = author.id === me.id || asRecord(ticket.proxyClientUser).id === me.id;
  const status = asString(ticket.status);
  const step = currentStep(t);
  const assigned = status === "in_progress" && step !== undefined && assigneeCanAct(step, me.id);
  const actionType = asString(asRecord(step).actionType);
  if (assigned) {
    actions.push("approve");
    if (actionType === "approve") actions.push("reject", "deny");
  }
  if (isAuthor && status === "in_progress") actions.push("withdraw");
  if ((isAuthor && ["draft", "rejected"].includes(status))
    || (me.permissions.includes("ticket_write") && ["in_progress", "completed", "denied"].includes(status))) {
    actions.push("archive");
  }
  if ((isAuthor && ["draft", "rejected"].includes(status)) || assigned) actions.push("edit");
  // A sub-status can forbid withdrawal, sending back and denial; the server
  // refuses them too, so the buttons are simply not offered.
  const subStatus = asRecord(ticket.subStatus);
  const forbidden: TicketAction[] = [];
  if (subStatus.allowWithdrawal === false) forbidden.push("withdraw");
  if (subStatus.allowRejection === false) forbidden.push("reject");
  if (subStatus.allowDenial === false) forbidden.push("deny");
  return actions.filter((action) => !forbidden.includes(action));
}

export function sendBackTargets(t: TicketDetail): { to: number; label: string }[] {
  const current = Math.max(0, Number(asRecord(t).currentStep ?? 0));
  return Array.from({ length: current }, (_, to) => ({ to, label: to === 0 ? "Applicant" : `Step ${to}` }));
}

export function stepSummary(step: TicketStep): string {
  const value = asRecord(step);
  const assignees = asArray(value.assignees);
  const completed = assignees.filter((assignee) => asRecord(assignee).completed === true).length;
  const required = Number(value.requiredApprovalsNumber ?? 1);
  const percent = Number(value.requiredApprovalsPercent ?? 0);
  const needed = percent > 0 ? Math.max(required, Math.ceil(assignees.length * percent / 100)) : required;
  const total = assignees.length || needed;
  const mode = needed < total ? "any" : "all";
  return `${completed}/${total} approved (${mode})`;
}
