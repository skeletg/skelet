// Installs the built package the way a project would and builds its template with a plugin - what catches a package
// that compiles here and doesn't work there: a dependency the list missed, a type that doesn't resolve outside this
// repo, a plugin's classes going unstyled. Run package/build.mjs first
//
//   node package/smoke.mjs [dir]     the project is left in dir (a temp one by default) to serve and look at
import { execFileSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const pkgDir = path.dirname(fileURLToPath(import.meta.url));
const out = path.join(pkgDir, "out");
const fail = (msg) => {
  console.error("dashboard package smoke: " + msg);
  process.exit(1);
};
if (!existsSync(path.join(out, "package.json"))) fail("package/out isn't built - run package/build.mjs first");

// A directory of its own: one this made before, or an empty or missing one - never somebody's files
const marker = ".skelet-dashboard-smoke";
const work = process.argv[2] ? path.resolve(process.argv[2]) : mkdtempSync(path.join(os.tmpdir(), "skelet-dashboard-smoke-"));
if (existsSync(work) && readdirSync(work).length > 0 && !existsSync(path.join(work, marker))) {
  fail(`${work} holds files this didn't put there - give it an empty directory, or none`);
}
rmSync(work, { recursive: true, force: true });
mkdirSync(work, { recursive: true });
writeFileSync(path.join(work, marker), "");
const run = (cmd, args, cwd) => execFileSync(cmd, args, { cwd, stdio: "inherit" });

// The tarball npm would publish, not the directory - what `files` leaves out has to be missing here too
run("npm", ["pack", "--silent", "--pack-destination", work], out);
const tarball = readdirSync(work).find((f) => f.endsWith(".tgz"));
const plugin = path.join(work, "plugin");
cpSync(path.join(pkgDir, "smoke", "plugin"), plugin, { recursive: true });

const project = path.join(work, "project");
cpSync(path.join(out, "template"), project, { recursive: true });
const edit = (file, change) => writeFileSync(path.join(project, file), change(readFileSync(path.join(project, file), "utf8")));
edit("package.json", (src) => {
  const manifest = JSON.parse(src);
  manifest.dependencies["@skelet/dashboard"] = "file:../" + tarball;
  manifest.dependencies["@skelet-smoke/dashboard-plugin"] = "file:../plugin";
  return JSON.stringify(manifest, null, 2) + "\n";
});
// The three places a plugin goes, as the template's own comments say
edit("src/plugins.ts", () => "import '@skelet-smoke/dashboard-plugin'\n\nexport {}\n");
edit("src/index.css", (src) => src + '@source "../node_modules/@skelet-smoke/dashboard-plugin/src";\n');
edit("vite.config.ts", (src) => {
  if (!src.includes("plugins: []")) fail("the template's vite.config.ts no longer has `plugins: []` to fill in");
  return src.replace("plugins: []", "plugins: ['@skelet-smoke/dashboard-plugin']");
});
writeFileSync(path.join(project, ".env"), "VITE_GRAPHQL_URL=https://api.example.com/api/admin/graphql\nVITE_STOREFRONT_ORIGIN=https://shop.example.com\n");

run("pnpm", ["install", "--prefer-offline", "--ignore-scripts"], project);
run("pnpm", ["exec", "tsc", "--noEmit"], project);
run("pnpm", ["exec", "vite", "build"], project);

const dist = path.join(project, "dist");
const assets = readdirSync(path.join(dist, "assets"));
const css = assets.filter((f) => f.endsWith(".css")).map((f) => readFileSync(path.join(dist, "assets", f), "utf8")).join("\n");
const js = assets.filter((f) => f.endsWith(".js")).map((f) => readFileSync(path.join(dist, "assets", f), "utf8")).join("\n");
const headers = readFileSync(path.join(dist, "_headers"), "utf8");
const checks = {
  "index.html": existsSync(path.join(dist, "index.html")),
  "the dashboard's public files": existsSync(path.join(dist, "favicon.svg")),
  "_redirects for the single-page app": existsSync(path.join(dist, "_redirects")),
  "the plugin's own classes in the CSS": css.includes("ff00aa") && css.includes("17px"),
  "the dashboard's classes in the CSS": css.includes("--sidebar"),
  "the plugin's page in the bundle": js.includes("smoke-warranties"),
  "the API's origin and its websocket in the CSP": headers.includes("connect-src 'self' https://api.example.com wss://api.example.com"),
  "the storefront framed for previews in the CSP": headers.includes("frame-src https://shop.example.com"),
  "the project's endpoint in the bundle": js.includes("https://api.example.com/api/admin/graphql"),
};
const missing = Object.entries(checks).filter(([, ok]) => !ok).map(([what]) => what);
if (missing.length > 0) fail("the built project lacks " + missing.join(", "));
console.log(`dashboard package smoke: a project on ${tarball} with a plugin installs, type-checks and builds - left in ${project}`);
