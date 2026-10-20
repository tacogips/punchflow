import { createResource, Show } from "solid-js";
import { useParams, A } from "@solidjs/router";
import { apiFetch } from "../../api/client";
import type { WorkflowDetail } from "../../api/types";
import JsonView, { ReadError } from "../../components/readviews/JsonView";
import PagedTable from "../../components/readviews/PagedTable";
export default function WorkflowView() {
  const params = useParams<{ id: string }>();
  const [workflow] = createResource(() => params.id, (id) => apiFetch<WorkflowDetail>("GET", `/v1/workflows/${encodeURIComponent(id)}`));
  const fields = () => (workflow()?.sectionList ?? []).flatMap((section) => section.formFields ?? []);
  return <main class="page"><p><A href="/workflows">Workflows</A></p><h1>Workflow</h1><Show when={workflow.error}><ReadError error={workflow.error} /></Show><Show when={workflow()}>{(value) => <>
    <dl class="summary"><dt>Name</dt><dd>{value().name}</dd><dt>Code</dt><dd>{String(value().code ?? "—")}</dd><dt>Status</dt><dd>{String(value().status ?? "—")}</dd><dt>Settings</dt><dd>{JSON.stringify(value().settings ?? {})}</dd></dl>
    <h2>Route branches</h2><PagedTable items={(value().routeConditions as Record<string, unknown>[] | undefined) ?? []} columns={[
      { key: "conditionType", label: "Condition" }, { key: "combinationType", label: "Combination" },
      { key: "route", label: "Route", value: (condition) => (condition.route as { name?: string } | undefined)?.name }, { key: "errorMessage", label: "Error message" },
    ]} />
    <h2>Form fields</h2><PagedTable items={fields()} columns={[{ key: "code", label: "Code" }, { key: "title", label: "Title" }, { key: "fieldType", label: "Type" }, { key: "required", label: "Required" }]} />
    <JsonView title="Workflow JSON" value={value()} />
  </>}</Show></main>;
}
