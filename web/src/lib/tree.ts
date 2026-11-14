export interface TreeNode<T> { item: T; children: TreeNode<T>[] }
export interface FlattenedTreeNode<T> { item: T; depth: number }

export function parentIndexFromChildLists(lists: Record<string, { id: string }[]>): Map<string, string | null> {
  const parents = new Map<string, string | null>();
  for (const [parent, children] of Object.entries(lists)) {
    for (const child of children) {
      const candidate = parent || null;
      const existing = parents.get(child.id);
      if (existing != null) continue;
      if (candidate !== null || !parents.has(child.id)) parents.set(child.id, candidate);
    }
  }
  return parents;
}

export function parentByPath<T>(
  items: T[],
  id: (item: T) => string,
  path: (item: T) => string | null | undefined,
  sep = "/",
): Map<string, string | null> {
  const idsByPath = new Map<string, string>();
  for (const item of items) {
    const itemPath = path(item);
    if (itemPath != null) idsByPath.set(itemPath, id(item));
  }

  const parents = new Map<string, string | null>();
  for (const item of items) {
    const itemPath = path(item);
    const separator = itemPath?.lastIndexOf(sep) ?? -1;
    const parentPath = separator > 0 ? itemPath?.slice(0, separator) : undefined;
    parents.set(id(item), parentPath ? idsByPath.get(parentPath) ?? null : null);
  }
  return parents;
}

export function buildTree<T>(items: T[], id: (item: T) => string, parentId: (item: T) => string | null): TreeNode<T>[] {
  const nodes = new Map<string, TreeNode<T>>();
  const parents = new Map<string, string | null>();
  for (const item of items) {
    const key = id(item);
    nodes.set(key, { item, children: [] });
    parents.set(key, parentId(item));
  }

  const roots: TreeNode<T>[] = [];
  for (const [key, node] of nodes) {
    const parentKey = parents.get(key);
    const parent = parentKey == null ? undefined : nodes.get(parentKey);
    if (!parent || parent === node || wouldCreateCycle(key, parentKey, parents)) roots.push(node);
    else parent.children.push(node);
  }
  return roots;
}

function wouldCreateCycle<T>(key: string, parentKey: string | null | undefined, parents: Map<string, string | null>): boolean {
  const seen = new Set<string>([key]);
  let current = parentKey;
  while (current !== null && current !== undefined) {
    if (seen.has(current)) return true;
    seen.add(current);
    current = parents.get(current);
  }
  return false;
}

export function flattenWithDepth<T>(nodes: TreeNode<T>[]): FlattenedTreeNode<T>[] {
  const result: FlattenedTreeNode<T>[] = [];
  const seen = new Set<TreeNode<T>>();
  const visit = (node: TreeNode<T>, depth: number) => {
    if (seen.has(node)) return;
    seen.add(node);
    result.push({ item: node.item, depth });
    for (const child of node.children) visit(child, depth + 1);
  };
  for (const node of nodes) visit(node, 0);
  return result;
}
