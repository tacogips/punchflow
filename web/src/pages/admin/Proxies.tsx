import { createResource, Show } from "solid-js";
import { apiList } from "../../api/client";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
type ProxyRegistration = { id: string; user?: { name?: string; email?: string }; proxyUser?: { name?: string; email?: string }; startsOn?: string | null; endsOn?: string | null; workflows?: { name?: string }[]; [key: string]: unknown };
export default function Proxies() {
  const [applicants] = createResource(() => apiList<ProxyRegistration>("/v1/proxyApplicants"));
  const [approvers] = createResource(() => apiList<ProxyRegistration>("/v1/proxyApprovers"));
  const person = (user?: ProxyRegistration["user"]) => user?.name ?? user?.email;
  return <main class="page"><h1>Proxy approvals</h1><section><h2>Applicants</h2><Show when={applicants.error}><ReadError error={applicants.error} /></Show><Show when={applicants()}>{(data) => <PagedTable items={data().items} columns={[
    { key: "user", label: "Applicant", value: (entry) => person(entry.user) }, { key: "proxyUser", label: "Proxy", value: (entry) => person(entry.proxyUser) },
    { key: "startsOn", label: "Starts" }, { key: "endsOn", label: "Ends" }, { key: "workflows", label: "Workflows", value: (entry) => entry.workflows?.map((workflow) => workflow.name).filter(Boolean).join(", ") },
  ]} />}</Show></section>
    <section><h2>Approvers</h2><Show when={approvers.error}><ReadError error={approvers.error} /></Show><Show when={approvers()}>{(data) => <PagedTable items={data().items} columns={[
      { key: "user", label: "Approver", value: (entry) => person(entry.user) }, { key: "proxyUser", label: "Proxy approver", value: (entry) => person(entry.proxyUser) },
      { key: "startsOn", label: "Starts" }, { key: "endsOn", label: "Ends" }, { key: "workflows", label: "Workflows", value: (entry) => entry.workflows?.map((workflow) => workflow.name).filter(Boolean).join(", ") },
    ]} />}</Show></section></main>;
}
