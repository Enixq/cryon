import { build } from "../frontend/node_modules/esbuild/lib/main.js";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
const assets = fileURLToPath(new URL("app/src/debug/assets/foreground-test/", import.meta.url));
await mkdir(assets, { recursive: true });
await build({
  entryPoints: [fileURLToPath(new URL("fixtures/foreground.tsx", import.meta.url))],
  bundle: true, platform: "browser", format: "iife", outfile: assets + "foreground.js",
  nodePaths: [fileURLToPath(new URL("../frontend/node_modules/", import.meta.url))],
  define: { "process.env.NODE_ENV": '"production"' },
});
await writeFile(assets + "index.html", '<html><head><meta name="viewport" content="width=device-width"></head><body><div id="root"></div><script src="foreground.js"></script></body></html>');
