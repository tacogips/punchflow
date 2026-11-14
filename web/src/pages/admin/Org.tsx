import { createResource, createSignal, For, Show } from "solid-js";
import { apiFetch, apiList } from "../../api/client";
import type { Team } from "../../api/types";
import { buildTree, parentIndexFromChildLists } from "../../lib/tree";
import Tree from "../../components/readviews/Tree";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
type Chart = { id: string; name: string; [key: string]: unknown };
type Member = { id?: string; user?: { id: string; name: string; email?: string }; name?: string; email?: string; leader?: boolean; grades?: { name?: string }[] };
export default function Org() {
  const [chartId, setChartId] = createSignal("");
  const [selectedTeam, setSelectedTeam] = createSignal<Team>();
  const [teamCache, setTeamCache] = createSignal<Record<string, Team[]>>({});
  const [childError, setChildError] = createSignal<unknown>();
  const [charts] = createResource(async () => {
    const listed = await apiList<Chart>("/v1/organizationCharts");
    if (!chartId()) { const current = await apiFetch<Chart>("GET", "/v1/organizationChart").catch(() => undefined); setChartId(current?.id ?? listed.items[0]?.id ?? ""); }
    return listed.items;
  });
  const [roots] = createResource(chartId, async (id) => {
    if (!id) return [];
    const data = await apiList<Team>(`/v1/organizationCharts/${encodeURIComponent(id)}/teams`);
    setTeamCache({ "": data.items });
    return data.items;
  });
  const teams = () => Object.values(teamCache()).flat();
  const [members] = createResource(() => [chartId(), selectedTeam()?.id] as const, ([chart, team]) => chart && team ? apiList<Member>(`/v1/organizationCharts/${encodeURIComponent(chart)}/teams/${encodeURIComponent(team)}/memberships`) : Promise.resolve({ items: [], pagination: { page: 1, perPage: 25, total: 0, links: {} } }));
  async function selectTeam(team: Team) {
    setSelectedTeam(team);
    const chart = chartId();
    if (teamCache()[team.id] || !chart) return;
    setChildError(undefined);
    try {
      const children = await apiList<Team>(`/v1/organizationCharts/${encodeURIComponent(chart)}/teams`, { parentId: team.id });
      if (chartId() !== chart) return;
      setTeamCache((current) => ({ ...current, [team.id]: children.items }));
    } catch (error) { setChildError(error); }
  }
  return <main class="page"><h1>Organization</h1><Show when={charts.error}><ReadError error={charts.error} /></Show><Show when={charts()}>{(items) => <>
    <label>Organization chart <select value={chartId()} onChange={(event) => { setChartId(event.currentTarget.value); setSelectedTeam(undefined); setTeamCache({}); setChildError(undefined); }}><For each={items()}>{(chart) => <option value={chart.id}>{chart.name}</option>}</For></select></label>
    <div class="org-layout"><section><h2>Teams</h2><Show when={roots.error}><ReadError error={roots.error} /></Show><Show when={childError()}><ReadError error={childError()} /></Show><Show when={roots()}><Tree nodes={buildTree(teams(), (team) => team.id, (team) => parentIndexFromChildLists(teamCache()).get(team.id) ?? null)} id={(team) => team.id} label={(team) => team.name} selectedId={selectedTeam()?.id} onSelect={(team) => void selectTeam(team)} /></Show></section>
    <section><h2>Members {selectedTeam() ? `· ${selectedTeam()?.name}` : ""}</h2><Show when={members.error}><ReadError error={members.error} /></Show><Show when={members()}>{(data) => <PagedTable items={data().items} columns={[
      { key: "member", label: "Member", value: (member) => member.user?.name ?? member.name }, { key: "email", label: "Email", value: (member) => member.user?.email ?? member.email },
      { key: "leader", label: "Leader" }, { key: "grade", label: "Grade", value: (member) => member.grades?.map((grade) => grade.name).filter(Boolean).join(", ") },
    ]} />}</Show></section></div>
  </>}</Show></main>;
}
