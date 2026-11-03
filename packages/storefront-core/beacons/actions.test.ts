import { beforeEach, describe, expect, it, vi } from "vitest";

const postBeacon = vi.fn();
vi.mock("./server", () => ({ postBeacon: (...a: unknown[]) => postBeacon(...a) }));

const { notifyBackInStockAction, submitSignupAction } = await import("./actions");

const form = (entries: Record<string, string>) => {
  const data = new FormData();
  for (const [k, v] of Object.entries(entries)) data.set(k, v);
  return data;
};
const productId = "1f8b64d2-96f1-4fbd-8252-f42bd396f9c1";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
beforeEach(() => {
  postBeacon.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
});

describe("notifyBackInStockAction", () => {
  it("posts the product and email, and succeeds on the backend's 204", async () => {
    postBeacon.mockResolvedValue(new Response(null, { status: 204 }));
    const result = await notifyBackInStockAction(undefined, form({ productId, email: "a@example.com" }));
    expect(postBeacon).toHaveBeenCalledWith("/t/back-in-stock", { productId, email: "a@example.com" });
    expect(result.status).toBe("success");
  });
  it("never reaches the backend with an invalid email", async () => {
    const result = await notifyBackInStockAction(undefined, form({ productId, email: "nope" }));
    expect(postBeacon).not.toHaveBeenCalled();
    expect(result.error?.email).toEqual(["email"]);
  });
  it("reports a transport failure", async () => {
    postBeacon.mockRejectedValue(new Error("down"));
    expect((await notifyBackInStockAction(undefined, form({ productId, email: "a@example.com" }))).error?.[""]).toEqual(["notifyFailed"]);
  });
});

describe("submitSignupAction", () => {
  it("returns the backend's success message", async () => {
    postBeacon.mockResolvedValue(json({ message: "Welcome aboard" }));
    const result = await submitSignupAction(undefined, form({ form: "newsletter", email: "a@example.com" }));
    expect(postBeacon).toHaveBeenCalledWith("/t/forms/submit", { form: "newsletter", email: "a@example.com", name: "" });
    expect(result).toMatchObject({ status: "success", message: "Welcome aboard" });
  });
  it("shows the backend's shopper-facing 400 sentinel, and words anything else itself", async () => {
    postBeacon.mockResolvedValueOnce(json({ error: "signup form not found" }, 400)).mockResolvedValueOnce(json({ error: "internal detail" }, 500));
    expect((await submitSignupAction(undefined, form({ form: "gone", email: "a@example.com" }))).error?.[""]).toEqual(["signup form not found"]);
    expect((await submitSignupAction(undefined, form({ form: "x", email: "a@example.com" }))).error?.[""]).toEqual(["signupFailed"]);
  });
});
