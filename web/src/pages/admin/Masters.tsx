import { createResource, createSignal, For, Show } from "solid-js";
import { apiList } from "../../api/client";
import type { GeneralMaster, GeneralMasterItem } from "../../api/types";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Masters() {
  const [masterId, setMasterId] = createSignal("");
  const [masters] = createResource(async () => { const data = await apiList<GeneralMaster>("/v1/generalMasters"); if (!masterId()) setMasterId(data.items[0]?.id ?? ""); return data.items; });
  const [items] = createResource(masterId, (id) => id ? apiList<GeneralMasterItem>(`/v1/generalMasters/${encodeURIComponent(id)}/items`) : undefined);
  return <main class="page"><h1>General masters</h1><Show when={masters.error}><ReadError error={masters.error} /></Show><Show when={masters()}>{(data) => <label>Master <select value={masterId()} onChange={(event) => setMasterId(event.currentTarget.value)}><For each={data()}>{(master) => <option value={master.id}>{master.name} ({master.code})</option>}</For></select></label>}</Show>
    <Show when={items.error}><ReadError error={items.error} /></Show><Show when={items()}>{(data) => <PagedTable items={data().items} columns={[{ key: "code", label: "Code" }, { key: "name", label: "Name" }, { key: "startsOn", label: "Starts on" }, { key: "endsOn", label: "Ends on" }, { key: "expired", label: "Status", value: (item) => item.expired || (item.endsOn && Date.parse(String(item.endsOn)) < Date.now()) ? "Expired" : "Active" }]} />}</Show>
  </main>;
}
