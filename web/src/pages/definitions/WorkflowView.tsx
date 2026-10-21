import { createResource, createSignal, For, Show } from "solid-js";
import { useParams, A } from "@solidjs/router";
import { apiFetch } from "../../api/client";
import type { WorkflowDetail } from "../../api/types";
import JsonView, { ReadError } from "../../components/readviews/JsonView";
import PagedTable from "../../components/readviews/PagedTable";
import { session } from "../../session";
const obj = (value: unknown): Record<string, unknown> => value && typeof value === "object" ? value as Record<string, unknown> : {};
const arr = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const str = (value: unknown): string => typeof value === "string" ? value : "";
export default function WorkflowView() {
  const params = useParams<{ id: string }>();
  const [workflow] = createResource(() => params.id, (id) => apiFetch<WorkflowDetail>("GET", `/v1/workflows/${encodeURIComponent(id)}`));
  const fields = () => (workflow()?.sectionList ?? []).flatMap((section) => section.formFields ?? []);
  const [notice, setNotice] = createSignal(""); const [error, setError] = createSignal(""); const [resetting, setResetting] = createSignal("");
  const resetCustomNumbering = async (numbering: Record<string, unknown>) => {
    const id = str(numbering.id);
    if (!id || !params.id || !confirm(`Reset ${str(numbering.name) || str(numbering.code)}? The next issued number will be 1.`)) return;
    setResetting(id); setError(""); setNotice("");
    try {
      await apiFetch("POST", `/ui/api/admin/workflows/${encodeURIComponent(params.id)}/custom-numberings/${encodeURIComponent(id)}/reset`);
      setNotice("Custom numbering counter reset. The next number will be 1.");
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not reset custom numbering counter."); }
    finally { setResetting(""); }
  };
  return <main class="page"><p><A href="/workflows">Workflows</A></p><h1>Workflow</h1><Show when={workflow.error}><ReadError error={workflow.error} /></Show><Show when={workflow()}>{(value) => <>
    <dl class="summary"><dt>Name</dt><dd>{value().name}</dd><dt>Code</dt><dd>{String(value().code ?? "—")}</dd><dt>Status</dt><dd>{String(value().status ?? "—")}</dd><dt>Settings</dt><dd>{JSON.stringify(value().settings ?? {})}</dd></dl>
    <h2>Route branches</h2><PagedTable items={(value().routeConditions as Record<string, unknown>[] | undefined) ?? []} columns={[
      { key: "conditionType", label: "Condition" }, { key: "combinationType", label: "Combination" },
      { key: "route", label: "Route", value: (condition) => (condition.route as { name?: string } | undefined)?.name }, { key: "errorMessage", label: "Error message" },
    ]} />
    <h2>Form fields</h2><PagedTable items={fields()} columns={[{ key: "code", label: "Code" }, { key: "title", label: "Title" }, { key: "fieldType", label: "Type" }, { key: "required", label: "Required" }]} />
    <Show when={session()?.permissions.includes("workflow") && arr(value().customNumberings).length > 0}><h2>Custom numbering</h2><Show when={notice()}><p role="status">{notice()}</p></Show><Show when={error()}><p role="alert">{error()}</p></Show><ul><For each={arr(value().customNumberings)}>{(item) => { const numbering = obj(item); return <li>{str(numbering.name) || str(numbering.code)} <button type="button" disabled={resetting() !== ""} onClick={() => void resetCustomNumbering(numbering)}>{resetting() === str(numbering.id) ? "Resetting…" : "Reset counter"}</button></li>; }}</For></ul></Show>
    <JsonView title="Workflow JSON" value={value()} />
  </>}</Show></main>;
}
