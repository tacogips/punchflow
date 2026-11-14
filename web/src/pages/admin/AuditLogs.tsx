import { createResource, createSignal, Show } from "solid-js";
import { apiList } from "../../api/client";
import type { AuditLog } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import JsonView, { ReadError } from "../../components/readviews/JsonView";
import { auditExportHref } from "../../lib/ticket-search";
import "../../components/notifications/notifications.css";
export default function AuditLogs() {
  const [page, setPage] = createSignal(1);
  const [userId, setUserId] = createSignal("");
  const [start, setStart] = createSignal("");
  const [end, setEnd] = createSignal("");
  const [filters, setFilters] = createSignal({ userId: "", createdAtStart: "", createdAtEnd: "" });
  const [result] = createResource(() => ({ page: page(), ...filters() }), ({ page: current, ...query }) => apiList<AuditLog>("/v1/auditLogs", { ...query, page: String(current), perPage: "25" }));
  return <main class="page"><h1>Audit logs</h1><form class="filters" onSubmit={(event) => { event.preventDefault(); setPage(1); setFilters({ userId: userId(), createdAtStart: start(), createdAtEnd: end() }); }}>
    <label>User ID <input value={userId()} onInput={(event) => setUserId(event.currentTarget.value)} /></label><label>Created from <input type="date" value={start()} onInput={(event) => setStart(event.currentTarget.value)} /></label><label>Created through <input type="date" value={end()} onInput={(event) => setEnd(event.currentTarget.value)} /></label><button type="submit">Filter</button>
  </form><p class="export-actions"><a class="button-link" href={auditExportHref(filters())} download="">Export CSV</a><span>Exports every log matching the filters above.</span></p><Show when={result.error}><ReadError error={result.error} /></Show><Show when={result()}>{(data) => <><PagedTable items={data().items} columns={[{ key: "createdAt", label: "Created" }, { key: "action", label: "Action" }, { key: "user", label: "User", value: (log) => (log.user as { name?: string; email?: string } | undefined)?.name }, { key: "systemType", label: "System" }, { key: "data", label: "Data", render: (log) => <JsonView title="View data" value={log.data} /> }]} total={data().pagination.total} page={data().pagination.page} perPage={data().pagination.perPage} onPage={setPage} rowKey={(log) => log.id} /></>}</Show></main>;
}
