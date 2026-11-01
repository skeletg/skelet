// `build` compiles the package it runs in; `pack <version> [dir]` packs all four and checks them as an outside project would
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const order = ["storefront-core", "storefront-react", "storefront-next", "storefront-astro"];
const relativeSpec = /(\bfrom\s*|\bimport\s*\(\s*|\bimport\s+)(["'])(\.{1,2}\/[^"']*)\2/g;
const skeletSpec = /(?:\bfrom\s*|\bimport\s*\(\s*|\bimport\s+)["'](@skelet\/storefront-[a-z]+)\/([^"']+)["']/g;
const directive = /^\s*["']use (client|server)["']/;
const directiveOf = (text) => directive.test(text.replace(/^(\s*(\/\/[^\n]*|\/\*[\s\S]*?\*\/))*/, ""));

const files = (dir) =>
  readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? files(path) : [path];
  });

const run = (cmd, args, opts) => {
  try {
    execFileSync(cmd, args, { stdio: "inherit", ...opts });
  } catch {
    fail([`${cmd} ${args.join(" ")} failed`], relative(root, opts.cwd) || "packages");
  }
};

const fail = (problems, what) => {
  if (problems.length === 0) return;
  console.error(`${what}:\n  ${problems.join("\n  ")}`);
  process.exit(1);
};

// tsc keeps imports as written, and Node and the bundlers both refuse an extensionless one inside an ESM package
function build(dir) {
  const dist = join(dir, "dist");
  rmSync(dist, { recursive: true, force: true });
  run("pnpm", ["exec", "tsc", "-p", "tsconfig.build.json"], { cwd: dir });
  const problems = [];
  for (const file of files(dist)) {
    const types = file.endsWith(".d.ts");
    if (!types && !file.endsWith(".js")) continue;
    const text = readFileSync(file, "utf8");
    const out = text.replace(relativeSpec, (match, lead, quote, spec) => {
      if (/\.(js|json)$/.test(spec)) return match;
      const target = resolve(dirname(file), spec);
      const ext = types ? ".d.ts" : ".js";
      if (existsSync(target + ext)) return `${lead}${quote}${spec}.js${quote}`;
      if (existsSync(join(target, `index${ext}`))) return `${lead}${quote}${spec}/index.js${quote}`;
      problems.push(`${relative(dir, file)} imports ${spec}, which isn't in dist`);
      return match;
    });
    if (out !== text) writeFileSync(file, out);
    if (types) continue;
    const source = [".ts", ".tsx"].map((ext) => join(dir, relative(dist, file)).replace(/\.js$/, ext)).find(existsSync);
    if (source && directiveOf(readFileSync(source, "utf8")) && !directiveOf(out)) {
      problems.push(`${relative(dir, file)} lost its "use" directive`);
    }
  }
  fail(problems, "build");
}

// The catalog pins one copy for the workspace, but a peer pinned exactly would refuse a project one patch ahead
function catalogPeers(peers) {
  const yaml = readFileSync(join(root, "..", "pnpm-workspace.yaml"), "utf8");
  const block = yaml.split(/^catalog:\n/m)[1]?.split(/^\S/m)[0] ?? "";
  const catalog = Object.fromEntries([...block.matchAll(/^\s+"?([^":\s]+)"?:\s*(\S+)/gm)].map(([, dep, range]) => [dep, range]));
  return Object.fromEntries(
    Object.entries(peers).map(([dep, range]) => [dep, range === "catalog:" && /^\d/.test(catalog[dep] ?? "") ? `^${catalog[dep]}` : range]),
  );
}

function pack(version, out) {
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version ?? "")) fail([`"${version}" isn't a version like 1.2.3`], "pack");
  out = resolve(out);
  rmSync(out, { recursive: true, force: true });
  mkdirSync(out, { recursive: true });
  const manifests = new Map(order.map((name) => [join(root, name, "package.json"), ""]));
  try {
    for (const path of manifests.keys()) {
      const text = readFileSync(path, "utf8");
      manifests.set(path, text);
      const manifest = JSON.parse(text);
      writeFileSync(path, JSON.stringify({ ...manifest, version, ...(manifest.peerDependencies && { peerDependencies: catalogPeers(manifest.peerDependencies) }) }, null, 2) + "\n");
    }
    for (const name of order) run("pnpm", ["pack", "--pack-destination", out], { cwd: join(root, name) });
  } finally {
    for (const [path, text] of manifests) if (text) writeFileSync(path, text);
  }
  verify(version, out);
  console.log(`${order.length} packages at ${version} in ${relative(process.cwd(), out) || "."}`);
}

function verify(version, out) {
  const scratch = mkdtempSync(join(tmpdir(), "skelet-npm-"));
  try {
    const packed = {};
    for (const tgz of readdirSync(out).filter((f) => f.endsWith(".tgz"))) {
      const dir = join(scratch, tgz);
      mkdirSync(dir);
      run("tar", ["-xzf", join(out, tgz), "-C", dir], { cwd: scratch });
      const manifest = JSON.parse(readFileSync(join(dir, "package", "package.json"), "utf8"));
      packed[manifest.name] = { dir: join(dir, "package"), manifest, tgz: join(out, tgz) };
    }
    const problems = [];
    for (const name of order.map((n) => `@skelet/${n}`)) {
      const pkg = packed[name];
      if (!pkg) {
        problems.push(`${name} wasn't packed`);
        continue;
      }
      const { dir, manifest } = pkg;
      if (manifest.version !== version) problems.push(`${name} is ${manifest.version}, not ${version}`);
      if (manifest.private) problems.push(`${name} is still private`);
      if (!manifest.exports?.["./*"]) problems.push(`${name} has no exports map`);
      if (!existsSync(join(dir, "LICENSE"))) problems.push(`${name} ships without its LICENSE`);
      for (const [dep, range] of Object.entries({ ...manifest.dependencies, ...manifest.peerDependencies })) {
        if (/^(workspace|catalog):/.test(range)) problems.push(`${name} depends on ${dep}@${range}`);
        if (dep.startsWith("@skelet/") && range !== version) problems.push(`${name} depends on ${dep}@${range}, not ${version}`);
        if (manifest.peerDependencies?.[dep] && /^\d/.test(range)) problems.push(`${name} pins its peer ${dep} to exactly ${range}`);
      }
      for (const file of files(dir)) {
        const rel = relative(dir, file);
        if (!rel.startsWith("dist/") && !["package.json", "README.md", "LICENSE"].includes(rel)) problems.push(`${name} ships ${rel}`);
        if (!rel.endsWith(".js")) continue;
        if (!existsSync(file.replace(/\.js$/, ".d.ts"))) problems.push(`${name} has no types for ${rel}`);
        const text = readFileSync(file, "utf8");
        for (const [, , , spec] of text.matchAll(relativeSpec)) {
          if (!existsSync(resolve(dirname(file), spec))) problems.push(`${name}/${rel} imports ${spec}, which isn't there`);
        }
        for (const [, dep, sub] of text.matchAll(skeletSpec)) {
          if (!existsSync(join(packed[dep]?.dir ?? "-", "dist", `${sub}.js`))) problems.push(`${name}/${rel} imports ${dep}/${sub}, which isn't there`);
        }
      }
    }
    fail(problems, "the packed tarballs");
    smoke(scratch, packed);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

function smoke(scratch, packed) {
  const app = join(scratch, "app");
  mkdirSync(app);
  writeFileSync(join(app, "package.json"), '{ "name": "smoke", "private": true, "type": "module" }\n');
  const core = packed["@skelet/storefront-core"];
  const react = packed["@skelet/storefront-react"];
  run("npm", ["install", "--no-audit", "--no-fund", "--ignore-scripts", "--loglevel=error", "--update-notifier=false", core.tgz, react.tgz, `react@${react.manifest.peerDependencies.react}`], { cwd: app });
  const modules = [core, react].flatMap(({ dir, manifest }) =>
    files(join(dir, "dist"))
      .filter((f) => f.endsWith(".js"))
      .map((f) => `${manifest.name}/${relative(join(dir, "dist"), f).replace(/\.js$/, "")}`),
  );
  const script = `const failed = []; for (const m of ${JSON.stringify(modules)}) { try { await import(m) } catch (e) { failed.push(m + ": " + e.message) } } if (failed.length) { console.error(failed.join("\\n")); process.exit(1) }`;
  run("node", ["--input-type=module", "-e", script], { cwd: app });
}

const [command, ...args] = process.argv.slice(2);
if (command === "build") build(process.cwd());
else if (command === "pack") pack(args[0], args[1] ?? join(root, ".packs"));
else fail([`usage: node publish.mjs build | pack <version> [dir]`], "publish.mjs");
