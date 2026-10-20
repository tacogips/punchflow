import { For, Show, createSignal } from "solid-js";
import type { TicketDetail } from "../../api/types";
import { availableActions, sendBackTargets } from "../../lib/ticket-view";

export default function ActionBar(props: {
  ticket: TicketDetail;
  me: { id: string; permissions: string[] };
  busy?: boolean;
  onAction: (action: "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit", data?: { comment?: string; to?: number }) => void;
}) {
  const [comment, setComment] = createSignal("");
  const [target, setTarget] = createSignal("0");
  const actions = () => availableActions(props.ticket, props.me);
  const act = (action: "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit") => {
    const data: { comment?: string; to?: number } = {};
    if (["approve", "reject", "deny"].includes(action) && comment().trim()) data.comment = comment().trim();
    if (action === "reject") data.to = Number(target());
    props.onAction(action, data);
  };
  return <section class="action-bar" aria-label="Ticket actions">
    <Show when={actions().includes("approve")}><label>Action comment <textarea value={comment()} onInput={(event) => setComment(event.currentTarget.value)} /></label>
      <button disabled={props.busy} onClick={() => act("approve")}>Approve</button>
      <Show when={actions().includes("reject")}><label>Send back to <select value={target()} onChange={(event) => setTarget(event.currentTarget.value)}><For each={sendBackTargets(props.ticket)}>{(item) => <option value={item.to}>{item.label}</option>}</For></select></label>
        <button disabled={props.busy} onClick={() => act("reject")}>Send back</button><button disabled={props.busy} onClick={() => act("deny")}>Deny</button>
      </Show>
    </Show>
    <Show when={actions().includes("withdraw")}><button disabled={props.busy} onClick={() => act("withdraw")}>Withdraw</button></Show>
    <Show when={actions().includes("archive")}><button disabled={props.busy} onClick={() => act("archive")}>Archive</button></Show>
    <Show when={actions().includes("edit")}><a class="button-link" href={`/tickets/${props.ticket.id}/edit`}>Edit</a></Show>
  </section>;
}
