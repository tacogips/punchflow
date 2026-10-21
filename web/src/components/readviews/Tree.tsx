import { For, Show } from "solid-js";
import type { TreeNode } from "../../lib/tree";

function Branch<T>(props: { nodes: TreeNode<T>[]; id: (item: T) => string; label: (item: T) => string; selectedId?: string; onSelect?: (item: T) => void }) {
  return <ul class="tree-list"><For each={props.nodes}>{(node) => <li>
    <button type="button" class="tree-node" aria-current={props.id(node.item) === props.selectedId ? "true" : undefined} onClick={() => props.onSelect?.(node.item)}>{props.label(node.item)}</button>
    <Show when={node.children.length}><Branch nodes={node.children} id={props.id} label={props.label} selectedId={props.selectedId} onSelect={props.onSelect} /></Show>
  </li>}</For></ul>;
}

export default function Tree<T>(props: { nodes: TreeNode<T>[]; id: (item: T) => string; label: (item: T) => string; selectedId?: string; onSelect?: (item: T) => void }) {
  return <nav class="tree" aria-label="Hierarchy"><Branch {...props} /></nav>;
}
