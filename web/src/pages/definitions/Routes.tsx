import { createResource, createSignal, Show } from "solid-js";
import { A } from "@solidjs/router";
import { apiList } from "../../api/client";
import type { Route } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Routes() {
  const [page, setPage] = createSignal(1);
  const [result] = createResource(page, (current) => apiList<Route>("/v1/routes", { page: String(current), perPage: "25" }));
  return <main class="page"><h1>Routes</h1><Show when={result.error}><ReadError error={result.error} /></Show><Show when={result()}>{(data) => <PagedTable items={data().items} columns={[
    { key: "code", label: "Code" }, { key: "name", label: "Name" }, { key: "status", label: "Status" },
    { key: "open", label: "", value: (item) => <A href={`/routes/${encodeURIComponent(item.id)}`}>View</A> },
  ]} total={data().pagination.total} page={data().pagination.page} perPage={data().pagination.perPage} onPage={setPage} />}</Show></main>;
}
