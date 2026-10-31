import type { Block } from "./blocks";
import { CmsBlocks } from "./CmsBlocks";

const COLS: Record<number, string> = {
  2: "sm:grid-cols-2",
  3: "sm:grid-cols-2 lg:grid-cols-3",
  4: "sm:grid-cols-2 lg:grid-cols-4",
};

export async function ColumnsBlock({ block }: { block: Block }) {
  const children = block.blocks ?? [];
  if (children.length === 0) return null;
  const count = Number(block.props.count) || 2;
  const cols = COLS[count] ?? COLS[2];

  return (
    <section className="border-b border-border">
      <div className={`mx-auto grid w-full max-w-8xl grid-cols-1 gap-6 px-6 md:px-10 py-10 ${cols}`}>
        {children.map((child) => (
          <div key={child.id}>
            <CmsBlocks blocks={[child]} />
          </div>
        ))}
      </div>
    </section>
  );
}
