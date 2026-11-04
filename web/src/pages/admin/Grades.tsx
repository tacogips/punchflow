import { createResource, Show } from "solid-js";
import { apiList } from "../../api/client";
import type { Grade } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Grades() {
  const [result] = createResource(() => apiList<Grade>("/v1/grades"));
  return <main class="page"><h1>Grades</h1><Show when={result.error}><ReadError error={result.error} /></Show><Show when={result()}>{(data) => <PagedTable items={data().items} columns={[{ key: "code", label: "Code" }, { key: "name", label: "Name" }, { key: "level", label: "Level" }, { key: "isDefault", label: "Default" }]} total={data().pagination.total} />}</Show></main>;
}
