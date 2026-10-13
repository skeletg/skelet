import { performance as nodePerformance } from "node:perf_hooks";
import { describe, expect, it, vi } from "vitest";
import { guardReactServerTiming } from "./react-server-timing";

const detail = { devtools: { trackGroup: "Server Components \u269b" } };

describe("React server timing guard", () => {
  it.each([-Infinity, -1])("handles an interrupted render ending at %s with the native API", (end) => {
    const target = { measure: nodePerformance.measure.bind(nodePerformance) as Performance["measure"] };
    guardReactServerTiming(target);
    const entry = target.measure("\u200bCheckoutPage", { start: 10, end, detail });
    expect(entry.startTime).toBe(10);
    expect(entry.duration).toBe(0);
    nodePerformance.clearMeasures(entry.name);
  });

  it("preserves valid React timings, legacy arguments, return values and receiver", () => {
    const result = {} as PerformanceMeasure;
    const measure = vi.fn<Performance["measure"]>().mockReturnValue(result);
    const target = { measure };
    guardReactServerTiming(target);
    const options = { start: 10, end: 20, detail };
    expect(target.measure("\u200bCheckoutPage", options)).toBe(result);
    expect(measure).toHaveBeenLastCalledWith("\u200bCheckoutPage", options, undefined);
    target.measure("navigation", "start", "end");
    expect(measure).toHaveBeenLastCalledWith("navigation", "start", "end");
    expect(measure.mock.contexts).toEqual([target, target]);
  });

  it.each([
    ["CheckoutPage", { start: 10, end: -Infinity, detail }],
    ["\u200bCheckoutPage", { start: 10, end: -Infinity }],
    ["\u200bCheckoutPage", { start: 10, end: -Infinity, detail: { devtools: { trackGroup: "Other" } } }],
  ])("keeps unrelated invalid measurements observable (%s)", (name, options) => {
    const target = { measure: nodePerformance.measure.bind(nodePerformance) as Performance["measure"] };
    guardReactServerTiming(target);
    expect(() => target.measure(name, options)).toThrow();
  });
});
