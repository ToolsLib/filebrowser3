import { describe, it, expect } from "vitest";
import { naturalCompare, sortResources } from "../sorting";
import { createPinia, setActivePinia } from "pinia";
import { useFileStore } from "@/stores/file";

describe("bounded listing state", () => {
  it("sorts natural names and preserves selected identities", () => {
    setActivePinia(createPinia());
    const store = useFileStore();
    const items = ["clip-10.txt", "clip-2.txt", "clip-1.txt"].map(
      (name, index) =>
        ({
          name,
          index,
          url: "/" + name,
          isDir: false,
          size: 1,
          modified: "2026-11-17",
        }) as ResourceItem
    );
    store.updateRequest({ items } as Resource);
    store.selected = [0, 2];
    const sorted = sortResources(items, { by: "name", asc: false });
    expect(sorted.map((i) => i.name)).toEqual([
      "clip-1.txt",
      "clip-2.txt",
      "clip-10.txt",
    ]);
    store.updateRequest({ items: sorted } as Resource);
    expect(store.selected).toEqual([0, 2]);
    expect(store.selectedSet.has(1)).toBe(false);
    expect(naturalCompare("a001b", "a1b")).toBe(0);
  });
});
