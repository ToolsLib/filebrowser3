import path from "node:path";
import { defineConfig, type PluginOption } from "vite";
import vue from "@vitejs/plugin-vue";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import { readdir, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { compression } from "vite-plugin-compression2";
const acePackage = JSON.parse(
  await readFile(
    createRequire(import.meta.url).resolve("ace-builds/package.json"),
    "utf8"
  )
);

const aceDir = path.dirname(
  createRequire(import.meta.url).resolve("ace-builds/package.json")
);
const themeSource = await readFile(
  path.join(aceDir, "src-noconflict/ext-themelist.js"),
  "utf8"
);
const themeData = JSON.parse(
  themeSource.match(/var themeData = (\[[\s\S]*?\]);/)![1]
) as string[][];
const themes = themeData.map(([caption, explicit, kind]) => {
  const name = explicit || caption.replace(/ /g, "_").toLowerCase();
  return { caption, name, theme: "ace/theme/" + name, isDark: kind === "dark" };
});
const plugins: PluginOption[] = [
  {
    name: "ace-metadata",
    resolveId(id: string) {
      if (id === "virtual:ace-themes") return "\0" + id;
    },
    load(id: string) {
      if (id === "\0virtual:ace-themes")
        return (
          "export const version=" +
          JSON.stringify(acePackage.version) +
          "; export const themes=" +
          JSON.stringify(themes) +
          "; export const themesByName=Object.fromEntries(themes.map(t=>[t.name,t]));"
        );
    },
  },
  vue(),
  VueI18nPlugin({
    include: [path.resolve(import.meta.dirname, "./src/i18n/**/*.json")],
  }),
  {
    name: "local-ace-assets",
    async generateBundle() {
      const dir = path.join(
        path.dirname(
          createRequire(import.meta.url).resolve("ace-builds/package.json")
        ),
        "src-min-noconflict"
      );
      for (const name of await readdir(dir)) {
        if (/^(mode|theme|worker)-.*\.js$/.test(name))
          this.emitFile({
            type: "asset",
            fileName: `ace/${acePackage.version}/${name}`,
            source: await readFile(path.join(dir, name)),
          });
      }
    },
  },
  compression({ include: /\.(js|css)$/, deleteOriginalAssets: false }),
];

const resolve = {
  alias: {
    // vue: "@vue/compat",
    "@/": `${path.resolve(import.meta.dirname, "src")}/`,
  },
};

// https://vitejs.dev/config/
export default defineConfig(({ command }) => {
  if (command === "serve") {
    return {
      plugins,
      resolve,
      server: {
        proxy: {
          "/api/command": {
            target: "ws://127.0.0.1:8080",
            ws: true,
          },
          "/api": "http://127.0.0.1:8080",
        },
      },
    };
  } else {
    // command === 'build'
    return {
      plugins,
      resolve,
      base: "",
      html: { cspNonce: "[{[ .CSPNonce ]}]" },
      build: {
        rollupOptions: {
          output: {
            // Go embed excludes leading underscores, including virtual-module chunks.
            chunkFileNames: (chunk) =>
              "assets/" + chunk.name.replace(/^_+/, "") + "-[hash].js",
          },
          input: {
            index: path.resolve(import.meta.dirname, "./public/index.html"),
          },
        },
      },
      experimental: {
        renderBuiltUrl(filename, { hostType }) {
          if (hostType === "js") {
            return { runtime: `window.__prependStaticUrl("${filename}")` };
          } else if (hostType === "html") {
            return `[{[ .StaticURL ]}]/${filename}`;
          } else {
            return { relative: true };
          }
        },
      },
    };
  }
});
