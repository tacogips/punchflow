import { For, Show } from "solid-js";
import type { TicketStep } from "../../api/types";
import { stepSummary, userLabel } from "../../lib/ticket-view";

const obj = (v: unknown): Record<string, unknown> => v && typeof v === "object" ? v as Record<string, unknown> : {};
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string => typeof v === "string" ? v : "";
export default function StepList(props: { steps: TicketStep[]; currentStep: number }) {
  return <ol class="step-list"><For each={props.steps}>{(step, index) => <li classList={{ "step-current": Number(obj(step).stepOrder ?? index() + 1) === props.currentStep, "step-complete": obj(step).completed === true }}>
    <strong>{str(obj(step).title) || `Step ${index() + 1}`}</strong><span>{str(obj(step).status) || stepSummary(step)}</span>
    <Show when={str(obj(step).instruction)}><p>{str(obj(step).instruction)}</p></Show>
    <ul><For each={arr(step.assignees)}>{(value) => {
      const assignee = obj(value); const proxies = arr(assignee.proxyUserIds).map(String);
      return <li>{userLabel(assignee.user) || "Unassigned"}{proxies.length ? ` (proxies: ${proxies.join(", ")})` : ""}{assignee.actualApprover ? ` · acted by ${userLabel(assignee.actualApprover)}` : ""}</li>;
    }}</For></ul>
  </li>}</For></ol>;
}
