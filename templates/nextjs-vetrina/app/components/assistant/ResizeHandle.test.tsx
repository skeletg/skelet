import { fireEvent, screen } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

import { MAX_W, ResizeHandle } from "./ResizeHandle";

const onCommit = vi.fn();
const onResizing = vi.fn();

function renderHandle(width = 520) {
  const panel = createRef<HTMLElement>();
  renderWithIntl(
    <aside ref={panel}>
      <ResizeHandle width={width} resizing={false} panel={panel} onResizing={onResizing} onCommit={onCommit} />
    </aside>,
  );
  return { panel, handle: screen.getByRole("separator") };
}

beforeEach(() => {
  Element.prototype.setPointerCapture = vi.fn();
});
afterEach(() => {
  vi.clearAllMocks();
  document.documentElement.removeAttribute("style");
});

describe("ResizeHandle", () => {
  it("widens and narrows the panel from the keyboard, within its bounds", () => {
    const { handle } = renderHandle();
    fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(onCommit).toHaveBeenLastCalledWith(536);
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(onCommit).toHaveBeenLastCalledWith(504);
    fireEvent.keyDown(handle, { key: "Enter" });
    expect(onCommit).toHaveBeenCalledTimes(2);
  });

  it("stops at the widest the panel goes", () => {
    const { handle } = renderHandle(MAX_W);
    fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(onCommit).toHaveBeenLastCalledWith(MAX_W);
  });

  it("follows a drag without React, and commits only where it ends", () => {
    const { panel, handle } = renderHandle();
    const root = document.documentElement;
    fireEvent.pointerDown(handle, { clientX: 1000, pointerId: 1 });
    expect(root.dataset.assistantResizing).toBe("");
    expect(onResizing).toHaveBeenLastCalledWith(true);
    fireEvent.pointerMove(handle, { clientX: 900, pointerId: 1 });
    expect(panel.current?.style.getPropertyValue("--assistant-w")).toBe("620px");
    expect(root.style.getPropertyValue("--assistant-inset")).toBe("min(620px, var(--assistant-max-share))");
    expect(onCommit).not.toHaveBeenCalled();
    fireEvent.pointerUp(handle, { clientX: 900, pointerId: 1 });
    expect(onCommit).toHaveBeenCalledWith(620);
    expect(onResizing).toHaveBeenLastCalledWith(false);
    expect(root.dataset.assistantResizing).toBeUndefined();
    expect(document.body.style.userSelect).toBe("");
    fireEvent.lostPointerCapture(handle, { pointerId: 1 });
    expect(onCommit).toHaveBeenCalledTimes(1);
  });
});
