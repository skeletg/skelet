"use client";
import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { Field } from "@/app/components/account/AccountUI";
import { useActionForm } from "@/app/lib/forms/use-action-form";
import { pickStoreProductAction, searchStoreProductsAction } from "@/app/lib/stores/actions";
import { productPickSchema, productSearchSchema } from "@/app/lib/stores/schemas";

export type PickedProduct = { id: string; name: string };

// Finds the product a store check is for: a simple product is picked outright, a configurable one through its variant
export function ProductPicker({ onPick, onClear }: { onPick: (product: PickedProduct | null) => void; onClear: () => void }) {
  const t = useTranslations("stores");
  const search = useActionForm(searchStoreProductsAction, productSearchSchema);
  const pick = useActionForm(pickStoreProductAction, productPickSchema);
  const hits = search.lastResult?.hits;
  const picked = pick.lastResult?.product;
  useEffect(() => {
    if (picked !== undefined) onPick(picked?.kind === "SIMPLE" ? { id: picked.id, name: picked.name || picked.sku } : null);
  }, [picked, onPick]);
  const failed = !!search.form.errors?.length || !!pick.form.errors?.length || pick.lastResult?.product === null;

  return <div className="space-y-3">
    <form id={search.form.id} action={search.formAction} onSubmit={search.onSubmit} noValidate className="flex items-start gap-2">
      <Field label={t("productSearch")} field={search.fields.q} maxLength={200} className="flex-1" />
      <button type="submit" className="mt-6 rounded-md border border-border p-2" disabled={search.pending}>{t("searchButton")}</button>
    </form>
    {(search.pending || pick.pending) && <p role="status">{t("loading")}</p>}
    {failed && <p role="alert">{t("productError")}</p>}
    {hits && <form id={pick.form.id} action={pick.formAction} onSubmit={pick.onSubmit} noValidate>
      <ul className="divide-y divide-border">
        {hits.map(hit => <li key={hit.id}><button type="submit" name="productId" value={hit.id} className="w-full py-2 text-left underline">{hit.name || hit.sku} · {hit.sku}</button></li>)}
      </ul>
    </form>}
    {hits?.length === 0 && <p>{t("noProducts")}</p>}
    {picked && <p className="font-medium">{picked.name || picked.sku}</p>}
    {picked?.kind === "CONFIGURABLE" && <label className="block text-sm">{t("variant")}
      <select key={picked.id} className="ml-2 rounded-md border border-border bg-background p-2" defaultValue="" onChange={e => {
        const child = picked.children.find(c => c.id === e.target.value);
        onPick(child ? { id: child.id, name: child.name || child.sku } : null);
      }}>
        <option value="">{t("selectVariant")}</option>
        {picked.children.filter(child => child.kind === "SIMPLE").map(child => <option key={child.id} value={child.id}>{child.name || child.sku} · {child.sku}</option>)}
      </select>
    </label>}
    {picked && !["SIMPLE", "CONFIGURABLE"].includes(picked.kind) && <p>{t("unsupported")}</p>}
    {picked && <button type="button" className="text-sm underline" onClick={onClear}>{t("clearProduct")}</button>}
  </div>;
}
