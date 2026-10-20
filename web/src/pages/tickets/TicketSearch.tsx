import { For, Show, createResource, createSignal } from "solid-js";
import { A } from "@solidjs/router";
import type { TicketWithStep, Workflow } from "../../api/types";
import { apiList } from "../../api/client";
import { formatJst } from "../../lib/format";

const statuses = ["draft", "in_progress", "rejected", "completed", "denied", "archived"];
type TicketQuery = Record<string, string | string[] | undefined>;
export default function TicketSearch() {
  const [status, setStatus] = createSignal<string[]>([]); const [workflowId, setWorkflowId] = createSignal(""); const [ticketNumber, setTicketNumber] = createSignal("");
  const [createdAtStart, setStart] = createSignal(""); const [createdAtEnd, setEnd] = createSignal(""); const [page, setPage] = createSignal(1); const [filters, setFilters] = createSignal<TicketQuery>({});
  const [workflows] = createResource(() => apiList<Workflow>("/v1/workflows", { perPage: "100" }).then((result) => result.items));
  const [result, { refetch }] = createResource(() => ({ ...filters(), page: page() }), ({ page: currentPage, ...query }) => apiList<TicketWithStep>("/v1/tickets", { ...query, page: String(currentPage), perPage: "25" }));
  const search = (event: SubmitEvent) => { event.preventDefault(); setPage(1); setFilters({ status: status(), workflowId: workflowId() || undefined, ticketNumber: ticketNumber().trim() || undefined, createdAtStart: createdAtStart() || undefined, createdAtEnd: createdAtEnd() || undefined }); };
  const move = (rel: "next" | "prev") => { const href = result()?.pagination.links[rel]; if (!href) return; const nextPage = Number(new URL(href, window.location.origin).searchParams.get("page")); if (nextPage > 0) setPage(nextPage); };
  return <main class="page"><header class="page-header"><h1>Ticket search</h1><A class="button-link" href="/tickets/new">Create ticket</A></header>
    <form class="filter-grid" onSubmit={search}><label>Statuses<For each={statuses}>{(value) => <span class="checkbox-option"><input type="checkbox" checked={status().includes(value)} onChange={(event) => setStatus((current) => event.currentTarget.checked ? [...current, value] : current.filter((item) => item !== value))} />{value.replaceAll("_", " ")}</span>}</For></label>
      <label>Workflow<select value={workflowId()} onChange={(event) => setWorkflowId(event.currentTarget.value)}><option value="">All workflows</option><For each={workflows() ?? []}>{(workflow) => <option value={workflow.id}>{workflow.name}</option>}</For></select></label>
      <label>Ticket number<input value={ticketNumber()} onInput={(event) => setTicketNumber(event.currentTarget.value)} /></label>
      <label>Created from<input type="date" value={createdAtStart()} onInput={(event) => setStart(event.currentTarget.value)} /></label>
      <label>Created through<input type="date" value={createdAtEnd()} onInput={(event) => setEnd(event.currentTarget.value)} /></label>
      <button>Search</button>
    </form>
    <Show when={!result.loading} fallback={<p>Loading tickets…</p>}><Show when={!result.error} fallback={<p role="alert">Unable to load tickets.</p>}>
      <p>{result()?.pagination.total ?? 0} tickets</p><ul class="ticket-list"><For each={result()?.items ?? []}>{(ticket) => <li>
        <A href={`/tickets/${ticket.id}`}><strong>{ticket.ticketNumber || "Untitled ticket"}</strong></A><span>{ticket.title}</span>
        <span class={`status status-${ticket.status}`}>{ticket.status.replaceAll("_", " ")}</span>
        <small>{ticket.createdAt ? formatJst(String(ticket.createdAt)) : ""}</small>
      </li>}</For></ul>
      <nav class="pagination" aria-label="Ticket pages"><button disabled={!result()?.pagination.links.prev} onClick={() => move("prev")}>Previous</button><span>Page {result()?.pagination.page}</span><button disabled={!result()?.pagination.links.next} onClick={() => move("next")}>Next</button></nav>
    </Show></Show>
    <button class="refresh-button" onClick={() => refetch()}>Refresh</button>
  </main>;
}
