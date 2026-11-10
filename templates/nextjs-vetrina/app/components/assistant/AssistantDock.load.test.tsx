import { act, fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

const chunk = vi.hoisted(() => ({ fail: true, tries: 0 }));
vi.mock("./AssistantPanel", () => {
  chunk.tries++;
  if (chunk.fail) throw new Error("Loading chunk 812 failed");
  return { AssistantPanel: ({ storeName }: { storeName: string }) => <p>Panel for {storeName}</p> };
});

const { AssistantProvider } = await import("./AssistantProvider");
const { AssistantTrigger } = await import("./AssistantTrigger");
const { AssistantDock } = await import("./AssistantDock");

it("says the assistant couldn't load instead of taking the page down, and fetches it again on a retry", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  renderWithIntl(
    <AssistantProvider enabled>
      <main>The page</main>
      <AssistantTrigger />
      <AssistantDock storeName="Workshop" />
    </AssistantProvider>,
  );
  fireEvent.pointerEnter(screen.getByRole("button", { name: "Ask the shopping assistant" }));
  fireEvent.click(screen.getByRole("button", { name: "Ask the shopping assistant" }));
  expect(screen.getByRole("status")).toHaveTextContent("Opening the assistant…");
  expect(await screen.findByRole("alert")).toHaveTextContent("The assistant couldn't load.");
  expect(screen.getByText("The page")).toBeInTheDocument();

  chunk.fail = false;
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Try again" })));
  expect(await screen.findByText("Panel for Workshop")).toBeInTheDocument();
  expect(chunk.tries).toBeGreaterThan(1);
});
