// Builds @skelet/dashboard-sdk: the declarations of src/sdk/index.ts, with the dashboard's @/ imports made relative so
// they resolve outside it. The dashboard's own build points the package name at the source, so this is types only
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const sdk = path.dirname(fileURLToPath(import.meta.url));
const app = path.dirname(sdk);
const dist = path.join(sdk, "dist");

rmSync(dist, { recursive: true, force: true });
execFileSync(path.join(app, "node_modules/.bin/tsc"), ["-p", path.join(app, "tsconfig.sdk.json")], { stdio: "inherit" });

const walk = (dir) =>
  readdirSync(dir).flatMap((name) => {
    const p = path.join(dir, name);
    return statSync(p).isDirectory() ? walk(p) : p.endsWith(".d.ts") ? [p] : [];
  });

for (const file of walk(dist)) {
  const src = readFileSync(file, "utf8");
  const out = src.replace(/(from\s+|import\(\s*)(["'])@\/([^"']+)\2/g, (_, lead, q, rest) => {
    let rel = path.relative(path.dirname(file), path.join(dist, rest)).split(path.sep).join("/");
    if (!rel.startsWith(".")) rel = "./" + rel;
    return `${lead}${q}${rel}${q}`;
  });
  if (out !== src) writeFileSync(file, out);
}

// vite-env.d.ts only declares the app's own build globals, and would make every consumer install vite's types
rmSync(path.join(dist, "vite-env.d.ts"), { force: true });

// The types reach these packages, so the SDK depends on them at the dashboard's own versions
const dash = JSON.parse(readFileSync(path.join(app, "package.json"), "utf8"));
const own = JSON.parse(readFileSync(path.join(sdk, "package.json"), "utf8"));
const versions = { ...dash.devDependencies, ...dash.dependencies };
const drift = Object.entries({ ...own.dependencies, ...own.peerDependencies }).filter(([n, v]) => versions[n] !== v);
if (drift.length > 0) {
  console.error("dashboard-sdk: sdk/package.json's versions differ from the dashboard's:", drift);
  process.exit(1);
}

const left = walk(dist).filter((f) => /(from\s+|import\(\s*)["']@\//.test(readFileSync(f, "utf8")));
if (left.length > 0) {
  console.error("dashboard-sdk: @/ imports left in", left);
  process.exit(1);
}
console.log(`dashboard-sdk: ${walk(dist).length} declaration files in ${path.relative(process.cwd(), dist)}`);
