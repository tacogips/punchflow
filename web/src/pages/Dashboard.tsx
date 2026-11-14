import { For, Show, createResource } from "solid-js";
import { A } from "@solidjs/router";
import type { TicketWithStep } from "../api/types";
import { apiList } from "../api/client";
import { session } from "../session";
import { formatJst } from "../lib/format";

const text = (value: unknown): string => typeof value === "string" ? value : "";
function TicketList(props: { tickets: TicketWithStep[] }) {
  return <ul class="ticket-list"><For each={props.tickets}>{(ticket) => <li>
    <A href={`/tickets/${ticket.id}`}><strong>{ticket.ticketNumber || ticket.title || "Untitled ticket"}</strong></A>
    <span>{ticket.title && ticket.ticketNumber ? ` · ${ticket.title}` : ""}</span>
    <span class={`status status-${ticket.status}`}>{ticket.status.replaceAll("_", " ")}</span>
    <small>{text(ticket.createdAt) ? formatJst(text(ticket.createdAt)) : ""}</small>
  </li>}</For></ul>;
}

export default function Dashboard() {
  const [mine] = createResource(() => session()?.user.id, (id) => id ? apiList<TicketWithStep>("/v1/tickets", { authorId: id, perPage: "10" }).then((result) => result.items) : Promise.resolve([]));
  const [tasks] = createResource(() => session()?.user.id, (id) => id ? apiList<TicketWithStep>("/v1/tickets/tasks", { perPage: "10" }).then((result) => result.items) : Promise.resolve([]));
  return <main class="page"><header class="page-header"><div><h1>Dashboard</h1><p>Welcome, {session()?.user.name}</p></div><A class="button-link" href="/tickets/new">Create ticket</A></header>
    <section><h2>My tickets</h2><Show when={!mine.loading} fallback={<p>Loading tickets…</p>}><Show when={!mine.error} fallback={<p role="alert">Unable to load your tickets.</p>}><TicketList tickets={mine() ?? []} /></Show></Show></section>
    <section><h2>Approval requests</h2><Show when={!tasks.loading} fallback={<p>Loading approval requests…</p>}><Show when={!tasks.error} fallback={<p role="alert">Unable to load approval requests.</p>}><TicketList tickets={tasks() ?? []} /></Show></Show></section>
  </main>;
}
