import type { Block } from "./blocks";
import { WebFormClient } from "./WebFormClient";
import type { InlineForm } from "./webform";

export function FormBlock({ block }: { block: Block }) {
  const form = block.props.form as InlineForm | undefined;
  if (!form || !Array.isArray(form.fields) || form.fields.length === 0) return null;
  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        {form.name && <h2 className="mb-6 text-2xl font-semibold">{form.name}</h2>}
        <WebFormClient form={form} />
      </div>
    </section>
  );
}
