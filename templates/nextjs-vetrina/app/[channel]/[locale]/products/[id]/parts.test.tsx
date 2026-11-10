import { screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import type { ProductAttributeValue } from "@skelet/storefront-next/catalog/read";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => ({ Link: ({ href, children }: { href: string; children: ReactNode }) => <a href={href}>{children}</a> }));
vi.mock("./Gallery", () => ({ ProductGallery: () => null }));

const { AttributeList, Description } = await import("./parts");

const doc = (...children: unknown[]) => ({ root: { type: "root", children } });

describe("Description", () => {
  it("draws Lexical as headings and lists", () => {
    renderWithIntl(
      <Description
        label="Description"
        value={doc({ type: "heading", tag: "h3", children: [{ type: "text", text: "Made to last" }] }, { type: "list", listType: "bullet", tag: "ul", children: [{ type: "listitem", children: [{ type: "text", text: "Stoneware" }] }] })}
      />,
    );
    expect(screen.getByRole("heading", { name: "Description" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Made to last" })).toBeInTheDocument();
    expect(screen.getByRole("listitem")).toHaveTextContent("Stoneware");
  });

  it("leaves out an editor someone emptied, heading and all", () => {
    const { container } = renderWithIntl(<Description label="Description" value={doc({ type: "paragraph", children: [] })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows plain copy as it was written, and nothing for blank copy", () => {
    renderWithIntl(<Description label="Description" value={"Soft cotton.\nMachine wash."} />);
    expect(screen.getByText(/Soft cotton\.\s+Machine wash\./)).toBeInTheDocument();
    const { container } = renderWithIntl(<Description label="Other" value="   " />);
    expect(container).toBeEmptyDOMElement();
  });
});

const av = (name: string, over: Partial<ProductAttributeValue>): ProductAttributeValue => ({
  attribute: { id: name, slug: name.toLowerCase(), name, unit: "", synapseTrack: false },
  selectedValues: [],
  scalarValue: null,
  references: [],
  ...over,
});

describe("AttributeList", () => {
  it("lists only the attributes this product fills in", () => {
    renderWithIntl(
      <AttributeList
        values={[
          av("Colour", { selectedValues: [{ id: "r", name: "Red", value: "red" }] }),
          av("Weight", { scalarValue: 320, attribute: { id: "w", slug: "weight", name: "Weight", unit: "g", synapseTrack: false } }),
          av("Brand", { references: [{ targetType: "brand", targetId: "b1", label: "Acme", url: "/brands/acme" }] }),
          av("Notes", { scalarValue: "" }),
          av("Material", {}),
        ]}
      />,
    );
    expect(screen.getAllByRole("term").map((t) => t.textContent)).toEqual(["Colour", "Weight", "Brand"]);
    expect(screen.getByRole("link", { name: "Acme" })).toHaveAttribute("href", "/brands/acme");
  });

  it("draws nothing when every attribute is blank", () => {
    const { container } = renderWithIntl(<AttributeList values={[av("Notes", { scalarValue: "" }), av("Material", {})]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
