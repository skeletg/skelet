import { act, fireEvent, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { expect, it, vi } from "vitest";

import { AssistantProvider, useAssistant } from "./AssistantProvider";

function Panel({ send }: { send: (text: string) => void }) {
  const { open, onAsk } = useAssistant();
  useEffect(() => {
    onAsk(send);
    return () => onAsk(null);
  }, [onAsk, send]);
  return <p>{open ? "open" : "closed"}</p>;
}

function Asker() {
  const { ask, trigger } = useAssistant();
  return (
    <button ref={trigger} type="button" onClick={() => ask("Tell me about the mug")}>
      ask
    </button>
  );
}

it("opens the panel and hands the message to whatever it registered to send with", () => {
  const send = vi.fn();
  render(
    <AssistantProvider enabled>
      <Asker />
      <Panel send={send} />
    </AssistantProvider>,
  );
  expect(screen.getByText("closed")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "ask" }));
  expect(screen.getByText("open")).toBeInTheDocument();
  expect(send).toHaveBeenCalledWith("Tell me about the mug");
});

it("stays shut on a store that doesn't show the assistant, and keeps hold of the header button", () => {
  let trigger: HTMLButtonElement | null = null;
  function Probe() {
    const a = useAssistant();
    useEffect(() => {
      trigger = a.trigger.current;
    });
    return null;
  }
  render(
    <AssistantProvider enabled={false}>
      <Asker />
      <Panel send={() => {}} />
      <Probe />
    </AssistantProvider>,
  );
  act(() => fireEvent.click(screen.getByRole("button", { name: "ask" })));
  expect(screen.getByText("closed")).toBeInTheDocument();
  expect(trigger).toBe(screen.getByRole("button", { name: "ask" }));
});
