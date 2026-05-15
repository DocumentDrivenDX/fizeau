import { copyFileSync, mkdirSync } from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { build } from "esbuild";

const require = createRequire(import.meta.url);
const root = process.cwd();
const duckdbDist = path.dirname(require.resolve("@duckdb/duckdb-wasm/dist/duckdb-browser-mvp.worker.js"));
const vendorDir = path.join(root, "website/static/vendor/duckdb");

mkdirSync(vendorDir, { recursive: true });

for (const file of [
  "duckdb-mvp.wasm",
  "duckdb-eh.wasm",
  "duckdb-browser-mvp.worker.js",
  "duckdb-browser-eh.worker.js",
]) {
  copyFileSync(path.join(duckdbDist, file), path.join(vendorDir, file));
}

await build({
  entryPoints: [path.join(root, "website/assets/js/benchmark-workbench.js")],
  outfile: path.join(root, "website/static/js/benchmark-workbench.js"),
  bundle: true,
  format: "esm",
  target: ["es2022"],
  legalComments: "none",
  logLevel: "info",
});
