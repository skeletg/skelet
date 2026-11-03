import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

const here = fileURLToPath(new URL(".", import.meta.url));
const core = fileURLToPath(new URL("../storefront-core/", import.meta.url));

const asyncExports = (src: string) => [...src.matchAll(/^export async function (\w+)/gm)].map((m) => m[1]).sort();

function serverModules(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    if (e.name === "node_modules") return [];
    const p = join(dir, e.name);
    if (e.isDirectory()) return serverModules(p);
    return /\.tsx?$/.test(e.name) && !e.name.includes(".test.") && readFileSync(p, "utf8").startsWith('"use server"') ? [p] : [];
  });
}

it("wraps every core action, and nothing else", () => {
  const modules = serverModules(here);
  expect(modules.length).toBeGreaterThan(10);
  for (const file of modules) {
    const src = readFileSync(file, "utf8");
    const from = src.match(/import \* as core from "@skelet\/storefront-core\/([\w/-]+)"/)?.[1];
    if (!from) continue;
    const wrapped = asyncExports(src);
    expect(wrapped, `${relative(here, file)} wraps ${from}`).toEqual(asyncExports(readFileSync(join(core, `${from}.ts`), "utf8")));
    for (const name of wrapped) expect(src, `${relative(here, file)}: ${name} calls the core's`).toContain(`core.${name}(`);
  }
});
