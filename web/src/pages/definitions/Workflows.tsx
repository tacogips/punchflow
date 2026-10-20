import { createResource, createSignal, Show } from "solid-js";
import { A } from "@solidjs/router";
import { apiList } from "../../api/client";
import type { Workflow } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Workflows() {
  const [page, setPage] = createSignal(1);
  const [result] = createResource(page, (current) => apiList<Workflow>("/v1/workflows", { page: String(current), perPage: "25" }));
  return <main class="page"><h1>Workflows</h1><Show when={result.error}><ReadError error={result.error} /></Show><Show when={result()}>
    {(data) => <PagedTable items={data().items} columns={[
      { key: "code", label: "Code" }, { key: "name", label: "Name" }, { key: "status", label: "Status" },
      { key: "folder", label: "Folder", value: (workflow) => {
        const folder = workflow.folder as { fullName?: string; name?: string } | null | undefined;
        return folder?.fullName ?? folder?.name;
      } },
    ]} total={data().pagination.total} page={data().pagination.page} perPage={data().pagination.perPage} onPage={setPage} rowActions={(item) => <A href={`/workflows/${encodeURIComponent(item.id)}`}>View</A>} />}
  </Show></main>;
}
