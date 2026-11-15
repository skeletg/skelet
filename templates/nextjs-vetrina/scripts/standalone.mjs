import { cpSync, existsSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const app = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const standalone = join(app, ".next", "standalone");
if (!existsSync(standalone)) process.exit(0);

let root = app;
while (!existsSync(join(root, "pnpm-workspace.yaml")) && dirname(root) !== root) root = dirname(root);
if (!existsSync(join(root, "pnpm-workspace.yaml"))) root = app;
const server = join(standalone, relative(root, app));

cpSync(join(app, ".next", "static"), join(server, ".next", "static"), { recursive: true });
if (existsSync(join(app, "public")) && readdirSync(join(app, "public")).length > 0) {
  cpSync(join(app, "public"), join(server, "public"), { recursive: true });
}
if (server !== standalone) {
  writeFileSync(join(standalone, "server.js"), `require("./${relative(standalone, server).split("\\").join("/")}/server.js");\n`);
}
console.log(`standalone server: ${relative(app, join(standalone, "server.js"))}`);
