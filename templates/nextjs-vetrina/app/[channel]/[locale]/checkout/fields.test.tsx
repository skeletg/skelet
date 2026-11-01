import { render, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useReturnToNextStep } from "./fields";

const mocks = vi.hoisted(() => ({ replace: vi.fn(), params: new URLSearchParams("offer=offer-1&step=details") }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: mocks.replace }), usePathname: () => "/checkout", useSearchParams: () => mocks.params }));

it("keeps the offer context when a saved step advances checkout", async () => {
  function SavedStep() { useReturnToNextStep({ status: "success" }); return null; }
  render(<SavedStep />);
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/checkout?offer=offer-1", { scroll: false }));
});
