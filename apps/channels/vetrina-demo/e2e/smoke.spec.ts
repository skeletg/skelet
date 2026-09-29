import { expect, test } from "@playwright/test";

import { main, open } from "./helpers";

// Every main page renders its content, throws nothing in the browser and trips no Content-Security-Policy rule.
// Media is left out: product images come from the object store's host port, which the test container can't reach
const PAGES = ["/", "/products", "/collections", "/casual-t-shirt", "/cart", "/blog", "/search?q=shirt", "/account/login"];

for (const path of PAGES) {
  test(`${path} renders without browser errors`, async ({ page }) => {
    const problems: string[] = [];
    page.on("pageerror", (err) => problems.push(`uncaught: ${err.message}`));
    page.on("console", (msg) => {
      const text = msg.text();
      if (msg.type() !== "error") return;
      if (/Content Security Policy|Refused to (load|execute|connect|apply)/.test(text)) problems.push(`csp: ${text}`);
    });

    await open(page, path);
    await expect(main(page)).toBeVisible();
    await expect(page.getByRole("status", { name: /Loading/ })).toHaveCount(0);
    expect(problems).toEqual([]);
  });
}
