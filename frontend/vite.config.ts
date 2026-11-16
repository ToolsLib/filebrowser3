import path from "node:path";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import { readdir, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { compression } from "vite-plugin-compression2";

const plugins = [
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
            fileName: `ace/${name}`,
            source: await readFile(path.join(dir, name)),
          });
      }
    },
  },
  compression({ include: /\.js$/, deleteOriginalAssets: false }),
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
