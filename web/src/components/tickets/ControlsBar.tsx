import { For, Show, createSignal } from "solid-js";
import type { CustomStepBody, TicketControls } from "../../api/types";
import { customStepBody, customStepProblem, hasAnyControl, raiseTargetLabel, type CustomStepForm } from "../../lib/ticket-controls";

const emptyForm = (): CustomStepForm => ({ title: "", actionType: "approve", assignmentType: "user", userIds: "", teamCodes: "", requiredApprovalsNumber: "", requiredApprovalsPercent: "", instruction: "", afterStepOrder: "" });

/**
 * Runtime lifecycle controls: cancel an approval, raise to a later step and add
 * an ad-hoc step. Which of them are offered is decided by the server.
 */
export default function ControlsBar(props: {
  controls: TicketControls;
  steps: { stepOrder?: unknown; title?: unknown }[];
  busy?: boolean;
  commentingEnabled?: boolean;
  onCancelApproval: (comment?: string) => void;
  onRaise: (to?: number) => void;
  onAddStep: (body: CustomStepBody) => void;
}) {
  const [target, setTarget] = createSignal("");
  const [open, setOpen] = createSignal(false);
  const [form, setForm] = createSignal<CustomStepForm>(emptyForm());
  const [problem, setProblem] = createSignal("");
  const [cancelComment, setCancelComment] = createSignal("");
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
    <Show when={props.controls.cancelApproval}><Show when={props.commentingEnabled}><label>Cancellation comment <textarea value={cancelComment()} onInput={(event) => setCancelComment(event.currentTarget.value)} /></label></Show><button type="button" disabled={props.busy} onClick={() => { props.onCancelApproval(cancelComment() || undefined); setCancelComment(""); }}>Cancel my approval</button></Show>
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
        <label>Action <select value={form().actionType} onChange={(event) => patch({ actionType: event.currentTarget.value })}><option value="approve">Approve / send back</option><option value="confirm">Circulate with confirmation</option><option value="none">Circulate without confirmation</option></select></label>
        <Show when={form().actionType !== "none"}><label>Assignment rule <select value={form().assignmentType} onChange={(event) => patch({ assignmentType: event.currentTarget.value as CustomStepForm["assignmentType"] })}><option value="user">Specify users</option><option value="team">Specify teams</option></select></label>
          <Show when={form().assignmentType === "user"} fallback={<label>Teams <input value={form().teamCodes} onInput={(event) => patch({ teamCodes: event.currentTarget.value })} placeholder="Team codes separated by commas" /></label>}>
            <label>Approvers <input value={form().userIds} onInput={(event) => patch({ userIds: event.currentTarget.value })} placeholder="User IDs separated by commas" /></label>
          </Show>
          <fieldset><legend>Next step threshold</legend><label>Required approval count <input type="number" min="1" step="1" value={form().requiredApprovalsNumber} onInput={(event) => patch({ requiredApprovalsNumber: event.currentTarget.value, requiredApprovalsPercent: "" })} /></label><label>Required approval percent <input type="number" min="1" max="100" step="1" value={form().requiredApprovalsPercent} onInput={(event) => patch({ requiredApprovalsPercent: event.currentTarget.value, requiredApprovalsNumber: "" })} /></label><small>Leave both blank to require every approver.</small></fieldset>
        </Show>
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
