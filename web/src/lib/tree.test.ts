import { describe, expect, test } from "bun:test";
import { buildTree, flattenWithDepth, parentByPath, parentIndexFromChildLists } from "./tree";

type Item = { id: string; parentId: string | null };
const id = (item: Item) => item.id;
const parent = (item: Item) => item.parentId;

describe("buildTree", () => {
  test("nests a flat list in depth-first order", () => {
    const items: Item[] = [
      { id: "root", parentId: null }, { id: "child", parentId: "root" }, { id: "leaf", parentId: "child" },
    ];
    const tree = buildTree(items, id, parent);
    expect(tree.map((node) => node.item.id)).toEqual(["root"]);
    expect(flattenWithDepth(tree).map(({ item, depth }) => [item.id, depth])).toEqual([["root", 0], ["child", 1], ["leaf", 2]]);
  });

  test("treats a missing parent as a root", () => {
    const tree = buildTree([{ id: "orphan", parentId: "missing" }], id, parent);
    expect(flattenWithDepth(tree).map(({ item, depth }) => [item.id, depth])).toEqual([["orphan", 0]]);
  });

  test("terminates cycles and includes each item once", () => {
    const tree = buildTree([{ id: "a", parentId: "b" }, { id: "b", parentId: "a" }], id, parent);
    const flattened = flattenWithDepth(tree);
    expect(flattened).toHaveLength(2);
    expect(flattened.map(({ item }) => item.id).sort()).toEqual(["a", "b"]);
  });
});

describe("tree parent indexes", () => {
  test("nests three levels from child-list keys", () => {
    const root = { id: "root" };
    const child = { id: "child" };
    const leaf = { id: "leaf" };
    const lists = { "": [root], root: [child], child: [leaf] };
    const parents = parentIndexFromChildLists(lists);
    const tree = buildTree([root, child, leaf], (item) => item.id, (item) => parents.get(item.id) ?? null);
    expect(flattenWithDepth(tree).map(({ item, depth }) => [item.id, depth])).toEqual([["root", 0], ["child", 1], ["leaf", 2]]);
  });

  test("nests folders from fullName paths and roots unmatched parents", () => {
    const folders = [
      { id: "root", fullName: "Root" },
      { id: "child", fullName: "Root/A" },
      { id: "leaf", fullName: "Root/A/B" },
      { id: "orphan", fullName: "Other/X" },
    ];
    const parents = parentByPath(folders, (item) => item.id, (item) => item.fullName);
    const tree = buildTree(folders, (item) => item.id, (item) => parents.get(item.id) ?? null);
    expect(flattenWithDepth(tree).map(({ item, depth }) => [item.id, depth])).toEqual([
      ["root", 0], ["child", 1], ["leaf", 2], ["orphan", 0],
    ]);
  });
});
