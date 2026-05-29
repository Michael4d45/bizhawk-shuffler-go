import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { transformAsync } from "@babel/core";
import type { BunPlugin } from "bun";

const pkgRoot = resolve(import.meta.dir, "..");
const defaultOutDir = resolve(pkgRoot, "../../serverhost/static");

export type BuildAdminOptions = {
  outDir?: string;
  minify?: boolean;
  clean?: boolean;
};

export async function buildAdminUI(options: BuildAdminOptions = {}): Promise<string> {
  const outDir = options.outDir ?? defaultOutDir;
  const assetsDir = join(outDir, "assets");
  const minify = options.minify ?? true;

  if (options.clean ?? true) {
    rmSync(outDir, { recursive: true, force: true });
  }
  mkdirSync(assetsDir, { recursive: true });

  const tailwindArgs = [
    "bunx",
    "@tailwindcss/cli",
    "-i",
    join(pkgRoot, "src/styles.css"),
    "-o",
    join(assetsDir, "index.css"),
  ];
  if (minify) tailwindArgs.push("--minify");

  const tailwind = Bun.spawn({
    cmd: tailwindArgs,
    cwd: pkgRoot,
    stdout: "inherit",
    stderr: "inherit",
  });
  if ((await tailwind.exited) !== 0) {
    throw new Error("tailwind build failed");
  }

  const reactCompilerPlugin: BunPlugin = {
    name: "react-compiler",
    setup(build) {
      build.onLoad({ filter: /\.(tsx|ts|jsx|js)$/ }, async (args) => {
        const code = await Bun.file(args.path).text();
        const result = await transformAsync(code, {
          filename: args.path,
          presets: [
            ["@babel/preset-typescript", { isTSX: true, allExtensions: true }],
            ["@babel/preset-react", { runtime: "automatic" }],
          ],
          plugins: [["babel-plugin-react-compiler", { target: "18" }]],
        });
        if (!result?.code) throw new Error(`babel transform failed: ${args.path}`);
        const loader = args.path.endsWith(".tsx") || args.path.endsWith(".jsx") ? "tsx" : "ts";
        return { contents: result.code, loader };
      });
    },
  };

  const buildResult = await Bun.build({
    entrypoints: [join(pkgRoot, "src/main.tsx")],
    outdir: assetsDir,
    minify,
    target: "browser",
    plugins: [reactCompilerPlugin],
    naming: minify ? "[name].[hash].[ext]" : "[name].[ext]",
  });

  if (!buildResult.success) {
    console.error(buildResult.logs);
    throw new Error("bun build failed");
  }

  const mainOut =
    buildResult.outputs.find((o) => basename(o.path).startsWith("main."))?.path ??
    join(assetsDir, "main.js");
  const mainName = basename(mainOut);

  writeFileSync(
    join(outDir, "index.html"),
    `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>BizShuffle Admin</title>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet" />
    <link rel="stylesheet" href="/assets/index.css" />
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/assets/${mainName}"></script>
  </body>
</html>
`
  );

  return outDir;
}
