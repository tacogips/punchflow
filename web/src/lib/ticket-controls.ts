import type { CustomStepBody, Label, SubStatus, TicketControls } from "../api/types";

/** Punchflow-only ticket operations live under /ui/api, not /v1. */
export const uiTicketPath = (ticketId: string, suffix = ""): string => `/ui/api/tickets/${encodeURIComponent(ticketId)}${suffix}`;

export const NO_CONTROLS: TicketControls = {
  cancelApproval: false, raise: false, raiseTargetStepOrders: [],
  addCustomStep: false, customStepInsertAfter: [], removableCustomStepIds: [],
};

/** Fills the fields a partial or missing controls response leaves out. */
export function normalizeControls(value: Partial<TicketControls> | null | undefined): TicketControls {
  return {
    cancelApproval: value?.cancelApproval === true,
    raise: value?.raise === true,
    raiseTargetStepOrders: Array.isArray(value?.raiseTargetStepOrders) ? value.raiseTargetStepOrders.map(Number) : [],
    addCustomStep: value?.addCustomStep === true,
    customStepInsertAfter: Array.isArray(value?.customStepInsertAfter) ? value.customStepInsertAfter.map(Number) : [],
    removableCustomStepIds: Array.isArray(value?.removableCustomStepIds) ? value.removableCustomStepIds.map(String) : [],
  };
}

export const hasAnyControl = (controls: TicketControls): boolean =>
  controls.cancelApproval || controls.raise || controls.addCustomStep;

export function parseUserIds(raw: string): string[] {
  return [...new Set(raw.split(/[\s,]+/).map((id) => id.trim()).filter(Boolean))];
}

export interface CustomStepForm { title: string; actionType: string; userIds: string; instruction: string; afterStepOrder: string }

/** Returns the first validation problem of the form, or null when it can be submitted. */
export function customStepProblem(form: CustomStepForm): string | null {
  if (!form.title.trim()) return "Enter a step title.";
  if (form.actionType !== "approve" && form.actionType !== "confirm") return "Choose approval or confirmation.";
  if (parseUserIds(form.userIds).length === 0) return "Enter at least one approver user ID.";
  return null;
}

export function customStepBody(form: CustomStepForm): CustomStepBody {
  const body: CustomStepBody = {
    title: form.title.trim(),
    actionType: form.actionType === "confirm" ? "confirm" : "approve",
    userIds: parseUserIds(form.userIds),
  };
  if (form.instruction.trim()) body.instruction = form.instruction.trim();
  if (form.afterStepOrder !== "") body.afterStepOrder = Number(form.afterStepOrder);
  return body;
}

/** Labels that can still be attached to a ticket. */
export function attachableLabels(all: Label[], attached: Label[]): Label[] {
  const taken = new Set(attached.map((label) => label.id));
  return all.filter((label) => !taken.has(label.id));
}

/** Picks a readable text color (black or white) for a label background such as "ef4444". */
export function labelTextColor(hex: string): "#111827" | "#ffffff" {
  const match = /^[0-9a-f]{6}$/i.exec(hex);
  if (!match) return "#111827";
  const channel = (offset: number) => parseInt(hex.slice(offset, offset + 2), 16);
  const luminance = (0.299 * channel(0) + 0.587 * channel(2) + 0.114 * channel(4)) / 255;
  return luminance > 0.6 ? "#111827" : "#ffffff";
}

/** Describes the actions a sub-status forbids, for display next to the sub-status. */
export function subStatusRestrictions(subStatus: Pick<SubStatus, "allowWithdrawal" | "allowRejection" | "allowDenial"> | null | undefined): string[] {
  if (!subStatus) return [];
  const out: string[] = [];
  if (subStatus.allowWithdrawal === false) out.push("withdrawal");
  if (subStatus.allowRejection === false) out.push("sending back");
  if (subStatus.allowDenial === false) out.push("denial");
  return out;
}

export function raiseTargetLabel(order: number, steps: { stepOrder?: unknown; title?: unknown }[]): string {
  const step = steps.find((candidate) => Number(candidate.stepOrder) === order);
  const title = typeof step?.title === "string" && step.title ? step.title : `Step ${order}`;
  return `${order}. ${title}`;
}
