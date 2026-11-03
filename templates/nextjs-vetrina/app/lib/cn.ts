// Joins class names, skipping falsy entries - `cn("a", on && "b", size === "sm" ? "c" : "d")`
export function cn(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(" ");
}
