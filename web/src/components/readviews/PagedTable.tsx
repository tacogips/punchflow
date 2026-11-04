import { For, Show } from "solid-js";
import type { JSX } from "solid-js";

export interface TableColumn<T> { key: string; label: string; value?: (item: T) => unknown; render?: (item: T) => JSX.Element }
export function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export default function PagedTable<T>(props: {
  items: T[]; columns: TableColumn<T>[]; page?: number; perPage?: number; total?: number;
  onPage?: (page: number) => void; rowActions?: (item: T) => JSX.Element; rowKey?: (item: T, index: number) => string;
}) {
  const page = () => props.page ?? 1;
  const perPage = () => props.perPage ?? 25;
  const total = () => props.total ?? props.items.length;
  const pages = () => Math.max(1, Math.ceil(total() / perPage()));
  return <>
    <div class="table-wrap"><table><thead><tr><For each={props.columns}>{(column) => <th scope="col">{column.label}</th>}</For><Show when={props.rowActions}><th scope="col">Actions</th></Show></tr></thead>
      <tbody><For each={props.items}>{(item, index) => <tr data-row-key={props.rowKey?.(item, index()) ?? index()}>
        <For each={props.columns}>{(column) => <td>{column.render ? column.render(item) : displayValue(column.value ? column.value(item) : (item as Record<string, unknown>)[column.key])}</td>}</For>
        <Show when={props.rowActions}><td>{props.rowActions?.(item)}</td></Show>
      </tr>}</For></tbody></table>
      <Show when={!props.items.length}><p class="empty-state">No records found.</p></Show>
    </div>
    <nav class="pagination" aria-label="Pagination"><span>{total()} records · Page {page()} of {pages()}</span>
      <button type="button" disabled={page() <= 1} onClick={() => props.onPage?.(page() - 1)}>Previous</button>
      <button type="button" disabled={page() >= pages()} onClick={() => props.onPage?.(page() + 1)}>Next</button>
    </nav>
  </>;
}
