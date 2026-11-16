import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { useFileStore } from "../../frontend/src/stores/file.ts";
const require = createRequire(
  new URL("../../frontend/package.json", import.meta.url),
);
const { createPinia, setActivePinia } = await import(
  pathToFileURL(require.resolve("pinia")).href
);
const makeResource = (n) => ({
  path: "/",
  isDir: true,
  items: Array.from({ length: n }, (_, i) => ({
    index: i,
    url: `/files/item-${i}.txt`,
    name: `item-${i}.txt`,
  })),
});
const median = (values) =>
  values.sort((a, b) => a - b)[Math.floor(values.length / 2)];
const prototypeUpdate = (store, value) => {
  const urls = new Set(store.selected.map((i) => store.req?.items[i]?.url));
  store.oldReq = store.req;
  store.req = value;
  store.selected = [];
  store.selected = store.req.items
    .filter((item) => urls.has(item.url))
    .map((item) => item.index);
};
const results = [];
for (const n of [100, 1000, 3000]) {
  const existing = [],
    prototype = [],
    unselected = [];
  for (let trial = 0; trial < 3; trial++) {
    for (const kind of ["unselected", "existing", "prototype"]) {
      setActivePinia(createPinia());
      const store = useFileStore();
      store.req = makeResource(n);
      store.selected =
        kind === "unselected" ? [] : Array.from({ length: n }, (_, i) => i);
      const next = makeResource(n),
        start = performance.now();
      if (kind === "prototype") prototypeUpdate(store, next);
      else store.updateRequest(next);
      ({ existing, prototype, unselected })[kind].push(
        performance.now() - start,
      );
      if (store.selectedCount !== (kind === "unselected" ? 0 : n))
        throw Error("Selection mismatch");
      store.$dispose();
    }
  }
  results.push({
    items: n,
    existingAllSelectedMedianMs: median(existing),
    setPrototypeAllSelectedMedianMs: median(prototype),
    existingNoneSelectedMedianMs: median(unselected),
  });
}
console.log(JSON.stringify({ node: process.version, results }, null, 2));
