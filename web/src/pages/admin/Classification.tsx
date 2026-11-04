import { createResource, Show } from "solid-js";
import { apiList } from "../../api/client";
import type { Category, Folder } from "../../api/types";
import { buildTree, parentByPath } from "../../lib/tree";
import Tree from "../../components/readviews/Tree";
import PagedTable from "../../components/readviews/PagedTable";
import { ReadError } from "../../components/readviews/JsonView";
export default function Classification() {
  const [categories] = createResource(() => apiList<Category>("/v1/categories"));
  const [folders] = createResource(() => apiList<Folder>("/v1/folders"));
  return <main class="page"><h1>Classification</h1><section><h2>Categories</h2><Show when={categories.error}><ReadError error={categories.error} /></Show><Show when={categories()}>{(data) => <PagedTable items={data().items} columns={[{ key: "code", label: "Code" }, { key: "name", label: "Name" }, { key: "description", label: "Description" }]} total={data().pagination.total} />}</Show></section>
    <section><h2>Folders</h2><Show when={folders.error}><ReadError error={folders.error} /></Show><Show when={folders()}>{(data) => {
      const items = data().items;
      const parents = parentByPath(items, (folder) => folder.id, (folder) => folder.fullName as string | undefined);
      return <Tree nodes={buildTree(items, (folder) => folder.id, (folder) => parents.get(folder.id) ?? null)} id={(folder) => folder.id} label={(folder) => folder.name} />;
    }}</Show></section></main>;
}
