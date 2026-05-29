import * as esbuild from "esbuild";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const outFile = join(here, "..", "ui", "vendor", "codemirror-yaml.bundle.js");

mkdirSync(dirname(outFile), { recursive: true });

await esbuild.build({
  entryPoints: [join(here, "codemirror-yaml-entry.js")],
  outfile: outFile,
  bundle: true,
  format: "iife",
  platform: "browser",
  target: ["es2020"],
  legalComments: "none",
  minify: true,
  sourcemap: false,
});

console.log("wrote", outFile);
