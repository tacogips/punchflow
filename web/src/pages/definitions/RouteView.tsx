import { createResource, Show } from "solid-js";
import { useParams, A } from "@solidjs/router";
import { apiFetch } from "../../api/client";
import type { RouteDetail, RouteStep } from "../../api/types";
import JsonView, { ReadError } from "../../components/readviews/JsonView";
import PagedTable from "../../components/readviews/PagedTable";
export default function RouteView() {
  const params = useParams<{ id: string }>();
  const [route] = createResource(() => params.id, (id) => apiFetch<RouteDetail>("GET", `/v1/routes/${encodeURIComponent(id)}`));
  const steps = () => (route()?.steps ?? []) as RouteStep[];
  return <main class="page"><p><A href="/routes">Routes</A></p><h1>Route</h1><Show when={route.error}><ReadError error={route.error} /></Show><Show when={route()}>{(value) => <>
    <dl class="summary"><dt>Name</dt><dd>{value().name}</dd><dt>Code</dt><dd>{String(value().code ?? "—")}</dd></dl>
    <h2>Steps</h2><PagedTable items={steps()} columns={[
      { key: "stepOrder", label: "Order" }, { key: "title", label: "Title" }, { key: "stepType", label: "Type" },
      { key: "actionType", label: "Action" }, { key: "approvers", label: "Approvers", value: (step) => step.users ?? step.targets }, { key: "condition", label: "Condition", value: (step) => step.routeStepCondition },
    ]} />
    <JsonView title="Route JSON" value={value()} />
  </>}</Show></main>;
}
