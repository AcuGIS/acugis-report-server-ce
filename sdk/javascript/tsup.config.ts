import { defineConfig } from "tsup";

export default defineConfig([
  {
    entry: { "acugis-report": "src/index.ts" },
    format: ["iife"],
    globalName: "AcuGIS",
    outDir: "dist",
    sourcemap: true,
    clean: true,
    target: "es2020",
    minify: false,
    dts: {
      entry: { index: "src/index.ts" },
    },
    outExtension() {
      return { js: ".js" };
    },
  },
  {
    entry: { "acugis-report.min": "src/index.ts" },
    format: ["iife"],
    globalName: "AcuGIS",
    outDir: "dist",
    sourcemap: true,
    clean: false,
    target: "es2020",
    minify: true,
    dts: false,
    outExtension() {
      return { js: ".js" };
    },
  },
  {
    entry: { "acugis-report.esm": "src/index.ts" },
    format: ["esm"],
    outDir: "dist",
    sourcemap: true,
    clean: false,
    target: "es2020",
    minify: false,
    dts: false,
    outExtension() {
      return { js: ".js" };
    },
  },
]);
