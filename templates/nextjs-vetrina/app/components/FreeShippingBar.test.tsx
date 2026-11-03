import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { renderWithIntl } from "@/app/test/render";
import { FreeShippingBar } from "./FreeShippingBar";
it("qualifies the threshold message because product-type rates can remain payable", () => {
  renderWithIntl(<FreeShippingBar subtotal={15000} thresholdCents={10000} currency="EUR" />);
  expect(screen.getByText(/threshold for eligible delivery rates/)).toBeVisible();
  expect(screen.getByText(/Product-specific charges may still apply/)).toBeVisible();
  expect(screen.queryByText(/unlocked free shipping/)).not.toBeInTheDocument();
});
