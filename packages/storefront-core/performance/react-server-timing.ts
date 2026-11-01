// React's bundled RSC dev profiler can leave end at -Infinity when a render is
// redirected, rejected or aborted. Keep its diagnostic measure from interrupting
// navigation. Remove once Next bundles the fix for react/react#37561.
export function guardReactServerTiming(target: Pick<Performance, "measure">): void {
  const measure = target.measure;
  target.measure = function (name, startOrOptions, endMark) {
    if (
      name.startsWith("\u200b") &&
      typeof startOrOptions === "object" &&
      startOrOptions !== null &&
      typeof startOrOptions.start === "number" &&
      typeof startOrOptions.end === "number" &&
      startOrOptions.end < 0 &&
      startOrOptions.detail?.devtools?.trackGroup === "Server Components \u269b"
    ) {
      // There is no usable end time; record a zero-duration diagnostic instead.
      return measure.call(this, name, { ...startOrOptions, end: startOrOptions.start });
    }
    return measure.call(this, name, startOrOptions, endMark);
  };
}
