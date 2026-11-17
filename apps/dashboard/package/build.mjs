// Builds the published @skelet/dashboard into package/out: the app compiled as a library (vite.lib.config.ts), the
// types of what plugins and projects import, the Tailwind source of its styles, its public files and CSP template,
// the Vite preset and the project template - and a package.json naming every package the compiled code imports
//
//   node package/build.mjs [version]     then: npm pack package/out
import { execFileSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const pkgDir = path.dirname(fileURLToPath(import.meta.url));
const app = path.dirname(pkgDir);
const repo = path.resolve(app, "../..");
const out = path.join(pkgDir, "out");
const dash = JSON.parse(readFileSync(path.join(app, "package.json"), "utf8"));
const version = process.argv[2] ?? dash.version;

const fail = (msg) => {
  console.error("dashboard package: " + msg);
  process.exit(1);
};
const bin = (name) => path.join(app, "node_modules", ".bin", name);
const walk = (dir, ext) =>
  readdirSync(dir).flatMap((name) => {
    const p = path.join(dir, name);
    return statSync(p).isDirectory() ? walk(p, ext) : p.endsWith(ext) ? [p] : [];
  });

rmSync(out, { recursive: true, force: true });
execFileSync(bin("vite"), ["build", "--config", "vite.lib.config.ts"], {
  cwd: app,
  stdio: "inherit",
  env: { ...process.env, SKELET_DASHBOARD_VERSION: version },
});

// Types - the sdk's and the config's, with the app's @/ imports made relative so they resolve outside it
execFileSync(bin("tsc"), ["-p", "tsconfig.package.json"], { cwd: app, stdio: "inherit" });
const types = path.join(out, "types");
for (const file of walk(types, ".d.ts")) {
  const src = readFileSync(file, "utf8");
  const rewritten = src.replace(/(from\s+|import\(\s*)(["'])@\/([^"']+)\2/g, (_, lead, q, rest) => {
    let rel = path.relative(path.dirname(file), path.join(types, rest)).split(path.sep).join("/");
    if (!rel.startsWith(".")) rel = "./" + rel;
    return `${lead}${q}${rel}${q}`;
  });
  if (rewritten !== src) writeFileSync(file, rewritten);
}
rmSync(path.join(types, "vite-env.d.ts"), { force: true });
if (walk(types, ".d.ts").some((f) => /(from\s+|import\(\s*)["']@\//.test(readFileSync(f, "utf8")))) fail("@/ imports left in the types");

// Everything else that ships
cpSync(path.join(app, "src", "index.css"), path.join(out, "styles.css"));
cpSync(path.join(app, "public"), path.join(out, "public"), { recursive: true });
cpSync(path.join(app, "headers.template"), path.join(out, "headers.template"));
for (const f of ["vite.js", "vite.d.ts", "index.d.ts", "README.md"]) cpSync(path.join(pkgDir, f), path.join(out, f));
cpSync(path.join(repo, "LICENSE"), path.join(out, "LICENSE"));
cpSync(path.join(repo, "PLUGIN-EXCEPTION.txt"), path.join(out, "PLUGIN-EXCEPTION.txt"));

// Every package the compiled code, the styles and the preset import - each at the dashboard's own version
const versions = { ...dash.devDependencies, ...dash.dependencies };
const name = (spec) => (spec.startsWith("@") ? spec.split("/").slice(0, 2).join("/") : spec.split("/")[0]);
const imported = new Set();
for (const file of walk(path.join(out, "dist"), ".js")) {
  const src = readFileSync(file, "utf8");
  // Statements at a line's start and import() calls - code inside the bundle's own strings says import too
  const specs = [
    ...src.matchAll(/^(?:import|export)\b[^;\n]*?\bfrom\s*["']([^"']+)["']/gm),
    ...src.matchAll(/^import\s*["']([^"']+)["']/gm),
    ...src.matchAll(/\bimport\(\s*["']([^"']+)["']\s*\)/g),
  ].map((m) => m[1]);
  for (const spec of specs) if (!spec.startsWith(".") && !spec.startsWith("/")) imported.add(name(spec));
}
for (const m of readFileSync(path.join(out, "styles.css"), "utf8").matchAll(/@import\s+["']([^"']+)["']/g)) imported.add(name(m[1]));
for (const m of readFileSync(path.join(out, "vite.js"), "utf8").matchAll(/from\s+["']([^"'.][^"']*)["']/g)) {
  if (!m[1].startsWith("node:")) imported.add(name(m[1]));
}
imported.add("tailwindcss"); // @tailwindcss/vite compiles styles.css with it
// Shared with the project and its plugins rather than owned, so all of them hold the one copy
const peers = ["react", "react-dom", "react-router-dom", "@apollo/client", "vite"];
const missing = [...imported].filter((n) => !versions[n]);
if (missing.length > 0) fail(`the build imports packages the dashboard doesn't declare: ${missing.join(", ")}`);
const pick = (names) => Object.fromEntries([...names].sort().map((n) => [n, versions[n]]));

writeFileSync(
  path.join(out, "package.json"),
  JSON.stringify(
    {
      name: "@skelet/dashboard",
      version,
      description: "The Skelet admin dashboard, compiled - a project builds it with its plugins' dashboard packages",
      license: "GPL-3.0-only WITH Skelet-Plugin-Exception",
      repository: { type: "git", url: "git+https://github.com/skeletg/skelet.git", directory: "apps/dashboard" },
      homepage: "https://github.com/skeletg/skelet/tree/main/apps/dashboard/package#readme",
      type: "module",
      sideEffects: ["*.css", "./dist/index.js"],
      exports: {
        ".": { types: "./index.d.ts", import: "./dist/index.js" },
        "./sdk": { types: "./types/sdk/index.d.ts", import: "./dist/sdk.js" },
        "./config": { types: "./types/lib/package/config.d.ts", import: "./dist/config.js" },
        "./vite": { types: "./vite.d.ts", import: "./vite.js" },
        "./styles.css": "./styles.css",
        "./package.json": "./package.json",
      },
      files: ["dist", "types", "public", "template", "styles.css", "headers.template", "vite.js", "vite.d.ts", "index.d.ts", "README.md", "LICENSE", "PLUGIN-EXCEPTION.txt"],
      dependencies: pick([...imported].filter((n) => !peers.includes(n))),
      peerDependencies: pick(peers),
    },
    null,
    2,
  ) + "\n",
);

// The project template, stamped with this version and the dashboard's own page shell
const template = path.join(out, "template");
cpSync(path.join(pkgDir, "template"), template, { recursive: true });
const html = readFileSync(path.join(app, "index.html"), "utf8");
if (!html.includes('src="/src/main.tsx"')) fail("index.html no longer loads /src/main.tsx - update the template stamping");
writeFileSync(path.join(template, "index.html"), html.replace('src="/src/main.tsx"', 'src="/src/main.ts"'));
const tpl = JSON.parse(readFileSync(path.join(template, "package.json"), "utf8"));
tpl.dependencies["@skelet/dashboard"] = "^" + version;
for (const p of peers.filter((p) => p !== "vite")) tpl.dependencies[p] = versions[p];
tpl.devDependencies = { typescript: versions.typescript, vite: versions.vite };
writeFileSync(path.join(template, "package.json"), JSON.stringify(tpl, null, 2) + "\n");

if (!existsSync(path.join(out, "dist", "index.js"))) fail("no dist/index.js");
mkdirSync(out, { recursive: true });
console.log(`dashboard package: @skelet/dashboard@${version} in ${path.relative(process.cwd(), out)}, ${imported.size} packages it imports`);
