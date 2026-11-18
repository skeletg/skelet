// Builds the published @skelet/dashboard into package/out: the app compiled as a library (vite.lib.config.ts), the
// types of what plugins and projects import, the Tailwind source of its styles, its public files and CSP template,
// the Vite preset and the project template - and a package.json naming every package the compiled code needs
//
//   node package/build.mjs [version]     then: npm pack package/out
//
// Without a version the package is marked private, so a build made to try something can't be published by mistake
import { execFileSync } from "node:child_process";
import { cpSync, existsSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { shared } from "./shared.js";

const pkgDir = path.dirname(fileURLToPath(import.meta.url));
const app = path.dirname(pkgDir);
const repo = path.resolve(app, "../..");
const out = path.join(pkgDir, "out");
const dash = JSON.parse(readFileSync(path.join(app, "package.json"), "utf8"));
const version = process.argv[2] ?? dash.version;
const releasing = process.argv[2] !== undefined;

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
const installed = (name) => JSON.parse(readFileSync(path.join(app, "node_modules", name, "package.json"), "utf8"));

// The package is the dashboard alone. A plugin in this tree, or one skelet gen listed for it, would be compiled in and
// shipped to every project
const pluginsDir = path.join(app, "src", "plugins");
const inTree = readdirSync(pluginsDir).filter((n) => statSync(path.join(pluginsDir, n)).isDirectory());
if (inTree.length > 0) fail(`src/plugins holds plugins of this tree (${inTree.join(", ")}) - build the package from a tree without them`);
if (/^\s*import\s/m.test(readFileSync(path.join(pluginsDir, "packages.ts"), "utf8"))) {
  fail("src/plugins/packages.ts imports plugin packages - build the package with no plugin enabled");
}

// Canaries where a maintainer's shell or .env.production would hold this deployment's settings: none may reach the
// output, where it would be every project's fallback endpoint
const canary = "skelet-build-canary-" + process.pid;
const settings = Object.fromEntries(
  ["VITE_GRAPHQL_URL", "VITE_FRONT_GRAPHQL_URL", "VITE_STOREFRONT_ORIGIN", "VITE_SENTRY_DSN"].map((k) => [k, `https://${canary}.invalid/${k}`]),
);

rmSync(out, { recursive: true, force: true });
execFileSync(bin("vite"), ["build", "--config", "vite.lib.config.ts"], {
  cwd: app,
  stdio: "inherit",
  env: { ...process.env, ...settings, SKELET_DASHBOARD_VERSION: version },
});
const dist = path.join(out, "dist");
if (!existsSync(path.join(dist, "index.js"))) fail("no dist/index.js");
const compiled = walk(dist, ".js").map((file) => ({ file, src: readFileSync(file, "utf8") }));
const baked = compiled.filter((c) => c.src.includes(canary)).map((c) => path.relative(out, c.file));
if (baked.length > 0) fail(`the build took settings from its environment into ${baked.join(", ")} - vite.lib.config.ts must read none`);

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
if (walk(types, ".d.ts").some((f) => /(from\s+|import\(\s*)["']@\//.test(readFileSync(f, "utf8")))) fail("@/ imports left in the types");

// The styles: the dashboard's Tailwind source, with the one stylesheet it takes from a command-line tool written in -
// a project shouldn't install that tool and all it depends on for a file - and told where its own class names are,
// relative to itself, so a project's stylesheet needn't know how its package manager laid node_modules out
const inlined = { 'shadcn/tailwind.css': path.join(app, "node_modules", "shadcn", "dist", "tailwind.css") };
let styles = readFileSync(path.join(app, "src", "index.css"), "utf8");
for (const [spec, file] of Object.entries(inlined)) {
  const line = `@import "${spec}";\n`;
  if (!styles.includes(line)) fail(`src/index.css no longer has ${line.trim()} - update what the package's styles inline`);
  // After the imports that stay, which have to come before any rule
  const rest = styles.replace(line, "");
  const imports = [...rest.matchAll(/^@import\s[^;]+;\n/gm)];
  const at = imports.length > 0 ? imports.at(-1).index + imports.at(-1)[0].length : 0;
  styles = `${rest.slice(0, at)}\n/* ${spec}, written in */\n${readFileSync(file, "utf8").trim()}\n${rest.slice(at)}`;
}
writeFileSync(path.join(out, "styles.css"), styles.trimEnd() + '\n\n@source "./dist";\n');

// Everything else that ships
const copied = ["vite.js", "vite.d.ts", "headers.js", "headers.d.ts", "shared.js", "shared.d.ts", "index.d.ts", "README.md"];
cpSync(path.join(app, "public"), path.join(out, "public"), { recursive: true });
cpSync(path.join(app, "headers.template"), path.join(out, "headers.template"));
for (const f of copied) cpSync(path.join(pkgDir, f), path.join(out, f));
cpSync(path.join(repo, "LICENSE"), path.join(out, "LICENSE"));
cpSync(path.join(repo, "PLUGIN-EXCEPTION.txt"), path.join(out, "PLUGIN-EXCEPTION.txt"));

// What the package needs installed: every package the compiled code, the styles and the preset import
const declared = { ...dash.devDependencies, ...dash.dependencies };
const name = (spec) => (spec.startsWith("@") ? spec.split("/").slice(0, 2).join("/") : spec.split("/")[0]);
const needed = new Set();
const specifiers = new Set(); // every package path the compiled code imports, for the preset's dev pre-bundling
for (const { src } of compiled) {
  // Statements at a line's start and import() calls - code inside the bundle's own strings says import too
  const specs = [
    ...src.matchAll(/^(?:import|export)\b[^;\n]*?\bfrom\s*["']([^"']+)["']/gm),
    ...src.matchAll(/^import\s*["']([^"']+)["']/gm),
    ...src.matchAll(/\bimport\(\s*["']([^"']+)["']\s*\)/g),
  ].map((m) => m[1]);
  for (const spec of specs) {
    if (spec.startsWith(".") || spec.startsWith("/")) continue;
    needed.add(name(spec));
    if (!/\.css$|\?/.test(spec)) specifiers.add(spec);
  }
}
writeFileSync(
  path.join(out, "optimize.js"),
  "// Generated by build.mjs: the package paths the compiled dashboard imports\nexport const imports = " + JSON.stringify([...specifiers].sort(), null, 2) + "\n",
);
for (const m of styles.matchAll(/@import\s+["']([^"']+)["']/g)) needed.add(name(m[1]));
for (const f of ["vite.js", "headers.js"]) {
  for (const m of readFileSync(path.join(out, f), "utf8").matchAll(/from\s+["']([^"'.][^"']*)["']/g)) {
    if (!m[1].startsWith("node:")) needed.add(name(m[1]));
  }
}
needed.add("tailwindcss"); // @tailwindcss/vite compiles styles.css with it
const undeclared = [...needed].filter((n) => !declared[n]);
if (undeclared.length > 0) fail(`the build imports packages the dashboard doesn't declare: ${undeclared.join(", ")}`);
// And what those need beside them: a package that asks for another as a peer, which the dashboard has for it, gets it
// here too - nothing imports it by name, and not every package manager installs a peer on its own
for (const n of needed) {
  const meta = installed(n);
  for (const peer of Object.keys(meta.peerDependencies ?? {})) {
    if (declared[peer] && !meta.peerDependenciesMeta?.[peer]?.optional) needed.add(peer);
  }
}

// Shared with the project and its plugins rather than owned, at the ranges the dashboard itself takes - everything
// else at the very version this build was compiled and tested against
const peers = [...shared, "vite"];
const range = (names) => Object.fromEntries([...names].sort().map((n) => [n, declared[n]]));
const exact = (names) => Object.fromEntries([...names].sort().map((n) => [n, installed(n).version]));

// The project template, stamped with this version and the dashboard's own page shell
const template = path.join(out, "template");
cpSync(path.join(pkgDir, "template"), template, { recursive: true });
const html = readFileSync(path.join(app, "index.html"), "utf8");
if (!html.includes('src="/src/main.tsx"')) fail("index.html no longer loads /src/main.tsx - update the template stamping");
writeFileSync(path.join(template, "index.html"), html.replace('src="/src/main.tsx"', 'src="/src/main.ts"'));
const tpl = JSON.parse(readFileSync(path.join(template, "package.json"), "utf8"));
tpl.dependencies = { "@skelet/dashboard": "^" + version, ...range(shared) };
tpl.devDependencies = range(["@types/react", "@types/react-dom", "typescript", "vite"]);
writeFileSync(path.join(template, "package.json"), JSON.stringify(tpl, null, 2) + "\n");

// Last, so a build that stopped anywhere above leaves nothing npm would pack
writeFileSync(
  path.join(out, "package.json"),
  JSON.stringify(
    {
      name: "@skelet/dashboard",
      version,
      ...(releasing ? {} : { private: true }),
      description: "The Skelet admin dashboard, compiled - a project builds it with its plugins' dashboard packages",
      license: "GPL-3.0-only WITH Skelet-Plugin-Exception",
      repository: { type: "git", url: "git+https://github.com/skeletg/skelet.git", directory: "apps/dashboard" },
      homepage: "https://github.com/skeletg/skelet/tree/main/apps/dashboard/package#readme",
      type: "module",
      exports: {
        ".": { types: "./index.d.ts", import: "./dist/index.js" },
        "./sdk": { types: "./types/sdk/index.d.ts", import: "./dist/sdk.js" },
        "./config": { types: "./types/lib/package/config.d.ts", import: "./dist/config.js" },
        "./vite": { types: "./vite.d.ts", import: "./vite.js" },
        "./styles.css": "./styles.css",
        "./package.json": "./package.json",
      },
      files: ["dist", "types", "public", "template", "styles.css", "headers.template", "optimize.js", ...copied, "LICENSE", "PLUGIN-EXCEPTION.txt"],
      dependencies: exact([...needed].filter((n) => !peers.includes(n))),
      peerDependencies: range(peers),
    },
    null,
    2,
  ) + "\n",
);

console.log(`dashboard package: @skelet/dashboard@${version}${releasing ? "" : " (private - no version given)"} in ${path.relative(process.cwd(), out)}, ${needed.size} packages it needs`);
