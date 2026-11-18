import { execFileSync, spawn } from "node:child_process";
import { copyFile, mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";
import { crc32 } from "node:zlib";
import UTIF from "utif";

// Store-only ZIP keeps the EPUB fixture self-contained and reproducible.
function zip(entries) {
  const bodies = [],
    directory = [];
  let offset = 0;
  for (const [name, text] of Object.entries(entries)) {
    const filename = Buffer.from(name),
      data = Buffer.from(text);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50);
    local.writeUInt16LE(20, 4);
    local.writeUInt32LE(crc32(data), 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(filename.length, 26);
    bodies.push(local, filename, data);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt32LE(crc32(data), 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(filename.length, 28);
    central.writeUInt32LE(offset, 42);
    directory.push(central, filename);
    offset += local.length + filename.length + data.length;
  }
  const table = Buffer.concat(directory),
    end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50);
  end.writeUInt16LE(Object.keys(entries).length, 8);
  end.writeUInt16LE(Object.keys(entries).length, 10);
  end.writeUInt32LE(table.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...bodies, table, end]);
}

const parent = path.resolve("test-results");
await mkdir(parent, { recursive: true });
const dir = await mkdtemp(path.join(parent, "server-"));
const root = path.join(dir, "files");
await mkdir(root);
await writeFile(
  path.join(root, "encoded-preview.csv"),
  Buffer.from("name;note\ncafé;value\n", "latin1")
);
await writeFile(
  path.join(root, "performance-markdown.md"),
  '# Markdown fixture\n\n$x^2$\n\n<img src="x" onerror="window.injected=true">\n'
);

await writeFile(
  path.join(root, "review.tiff"),
  Buffer.from(
    UTIF.encodeImage(
      new Uint8Array([
        255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255,
      ]).buffer,
      2,
      2
    )
  )
);
await mkdir(path.join(root, "Large folder"));
await Promise.all(
  Array.from({ length: 250 }, (_, index) =>
    writeFile(
      path.join(
        root,
        "Large folder",
        `item-${String(index).padStart(3, "0")}.txt`
      ),
      "Pagination fixture"
    )
  )
);
await writeFile(
  path.join(root, "large-preview.csv"),
  "a,b,c,d,e,f,g,h\n" +
    Array.from({ length: 20000 }, (_, i) => `${i},a,b,c,d,e,f,g\n`).join("")
);
await writeFile(
  path.join(root, "oversized-preview.csv"),
  "a,b\n" + "x,y\n".repeat(1500000)
);

for (const name of ["Documents", "Projects", "Photos", "Empty folder"])
  await mkdir(path.join(root, name));
await writeFile(
  path.join(root, "Documents", "Meeting notes.md"),
  "# Meeting notes\n\nA simple place for shared notes.\n"
);
await writeFile(
  path.join(root, "Projects", "Project brief.md"),
  "# Project brief\n\nKeep the work focused and easy to find.\n"
);
await writeFile(
  path.join(root, "Quarterly report.csv"),
  "quarter,revenue\nQ1,24000\nQ2,28000\n"
);
await writeFile(
  path.join(root, "Getting started.txt"),
  "Welcome to your workspace.\n"
);
await writeFile(
  path.join(root, "Résumé — design notes.txt"),
  "Unicode filenames stay readable.\n"
);
await writeFile(
  path.join(root, "review.md"),
  "# Review fixture\n\nOriginal text.\n"
);
await writeFile(path.join(root, "review.csv"), "name,value\nreview,42\n");
await writeFile(
  path.join(root, "review.epub"),
  zip({
    mimetype: "application/epub+zip",
    "META-INF/container.xml": `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="book.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
    "book.opf": `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">review-fixture</dc:identifier><dc:title>Review book</dc:title><dc:language>en</dc:language><meta property="dcterms:modified">2026-11-16T00:00:00Z</meta></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/></manifest><spine><itemref idref="chapter"/></spine></package>`,
    "nav.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head><body><nav epub:type="toc"><ol><li><a href="chapter.xhtml">Chapter</a></li></ol></nav></body></html>`,
    "chapter.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter</title></head><body><h1>Offline EPUB chapter</h1><p>Review content.</p></body></html>`,
  })
);
await copyFile(
  path.resolve("public/img/icons/android-chrome-192x192.png"),
  path.join(root, "review.png")
);
const binary = path.resolve(
  "..",
  process.platform === "win32" ? "filebrowser.exe" : "filebrowser"
);
const password = execFileSync(binary, ["hash", "Browser-test-password-42!"], {
  encoding: "utf8",
}).trim();
const server = spawn(
  binary,
  [
    "--root",
    root,
    "--database",
    path.join(dir, "test.db"),
    "--username",
    "reviewer",
    "--password",
    password,
    "--port",
    "18089",
    "--insecureCookies",
    "--baseURL",
    "/test-app",
  ],
  {
    stdio: "inherit",
    windowsHide: true,
    env: { ...process.env, FB_MAX_UPLOAD_SIZE: "107374182400" },
  }
);
for (const signal of ["SIGINT", "SIGTERM"])
  process.on(signal, () => server.kill(signal));
server.on("exit", (code) => process.exit(code ?? 0));
