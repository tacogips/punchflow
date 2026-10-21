import { For, Show, createSignal } from "solid-js";
import type { CustomStepBody, TicketControls } from "../../api/types";
import { customStepBody, customStepProblem, hasAnyControl, raiseTargetLabel, type CustomStepForm } from "../../lib/ticket-controls";

const emptyForm = (): CustomStepForm => ({ title: "", actionType: "approve", userIds: "", instruction: "", afterStepOrder: "" });

/**
 * Runtime lifecycle controls: cancel an approval, raise to a later step and add
 * an ad-hoc step. Which of them are offered is decided by the server.
 */
export default function ControlsBar(props: {
  controls: TicketControls;
  steps: { stepOrder?: unknown; title?: unknown }[];
  busy?: boolean;
  onCancelApproval: () => void;
  onRaise: (to?: number) => void;
  onAddStep: (body: CustomStepBody) => void;
}) {
  const [target, setTarget] = createSignal("");
  const [open, setOpen] = createSignal(false);
  const [form, setForm] = createSignal<CustomStepForm>(emptyForm());
  const [problem, setProblem] = createSignal("");
  const patch = (change: Partial<CustomStepForm>) => setForm((current) => ({ ...current, ...change }));
  const submit = (event: Event) => {
    event.preventDefault();
    const issue = customStepProblem(form());
    setProblem(issue ?? "");
    if (issue) return;
    props.onAddStep(customStepBody(form()));
    setForm(emptyForm()); setOpen(false);
  };
  const chosenTarget = () => target() === "" ? props.controls.raiseTargetStepOrders[0] : Number(target());
  return <Show when={hasAnyControl(props.controls)}><section class="action-bar controls-bar" aria-label="Approval controls">
    <Show when={props.controls.cancelApproval}><button type="button" disabled={props.busy} onClick={() => props.onCancelApproval()}>Cancel my approval</button></Show>
    <Show when={props.controls.raise}>
      <Show when={props.controls.raiseTargetStepOrders.length > 1}>
        <label>Raise to <select value={target()} onChange={(event) => setTarget(event.currentTarget.value)}><For each={props.controls.raiseTargetStepOrders}>{(order) => <option value={order}>{raiseTargetLabel(order, props.steps)}</option>}</For></select></label>
      </Show>
      <button type="button" disabled={props.busy} onClick={() => props.onRaise(props.controls.raiseTargetStepOrders.length > 1 ? chosenTarget() : undefined)}>Raise to my step</button>
    </Show>
    <Show when={props.controls.addCustomStep}>
      <button type="button" disabled={props.busy} aria-expanded={open()} onClick={() => setOpen(!open())}>Add step</button>
      <Show when={open()}><form class="custom-step-form" onSubmit={submit}>
        <label>Step title <input value={form().title} maxLength={100} onInput={(event) => patch({ title: event.currentTarget.value })} /></label>
        <label>Action <select value={form().actionType} onChange={(event) => patch({ actionType: event.currentTarget.value })}><option value="approve">Approve / send back</option><option value="confirm">Circulate with confirmation</option></select></label>
        <label>Approvers <input value={form().userIds} onInput={(event) => patch({ userIds: event.currentTarget.value })} placeholder="User IDs separated by commas" /></label>
        <label>Instruction <input value={form().instruction} onInput={(event) => patch({ instruction: event.currentTarget.value })} /></label>
        <Show when={props.controls.customStepInsertAfter.length > 1}>
          <label>Insert after <select value={form().afterStepOrder} onChange={(event) => patch({ afterStepOrder: event.currentTarget.value })}><option value="">Current step</option><For each={props.controls.customStepInsertAfter}>{(order) => <option value={order}>{raiseTargetLabel(order, props.steps)}</option>}</For></select></label>
        </Show>
        <Show when={problem()}><p role="alert">{problem()}</p></Show>
        <button type="submit" disabled={props.busy}>Add step</button>
      </form></Show>
    </Show>
  </section></Show>;
}
