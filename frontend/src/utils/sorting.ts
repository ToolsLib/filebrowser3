// Natural decimal ordering follows the backend's maruel/natural rules,
// including equal numeric runs followed by text and terminal leading zeros.
function lexical(a: string, b: string): number {
  const aa = Array.from(a),
    bb = Array.from(b);
  for (let i = 0; i < Math.min(aa.length, bb.length); i++) {
    const d = aa[i].codePointAt(0)! - bb[i].codePointAt(0)!;
    if (d) return d;
  }
  return aa.length - bb.length;
}
export function naturalCompare(a: string, b: string): number {
  while (true) {
    let p = 0;
    while (p < a.length && p < b.length && a[p] === b[p] && !/[0-9]/.test(a[p]))
      p++;
    a = a.slice(p);
    b = b.slice(p);
    if (!a || !b) return lexical(a, b);
    const an = a.match(/^\d+/)?.[0],
      bn = b.match(/^\d+/)?.[0];
    if (an && bn) {
      const x = an.replace(/^0+/, "") || "0",
        y = bn.replace(/^0+/, "") || "0";
      const d = x.length - y.length || lexical(x, y);
      if (d) return d;
      if (an.length < a.length && bn.length < b.length) {
        a = a.slice(an.length);
        b = b.slice(bn.length);
        continue;
      }
    }
    return lexical(a, b);
  }
}
export function sortResources(
  items: ResourceItem[],
  sorting: Sorting
): ResourceItem[] {
  const names = new Map(items.map((item) => [item, item.name.toLowerCase()]));
  return [...items]
    .sort((a, b) => {
      if (sorting.by === "name") {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        const d = naturalCompare(names.get(a)!, names.get(b)!);
        return sorting.asc ? -d : d;
      }
      const d =
        sorting.by === "size"
          ? (a.isDir ? -2147483648 : 0) +
            a.size -
            ((b.isDir ? -2147483648 : 0) + b.size)
          : Date.parse(a.modified) - Date.parse(b.modified);
      return sorting.asc ? d : -d;
    })
    .map((item, index) => ({ ...item, index }));
}
