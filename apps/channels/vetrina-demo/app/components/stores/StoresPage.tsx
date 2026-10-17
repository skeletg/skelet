"use client";
import { useState } from "react";
import { ProductPicker, type PickedProduct } from "./ProductPicker";
import { StoreFinder } from "./StoreFinder";

// Product search only where stock can be checked; flat inventory keeps the plain store directory
export function StoresPage({ stockChecking }: { stockChecking: boolean }) {
  const [product, setProduct] = useState<PickedProduct | null>(null);
  const [pickerKey, setPickerKey] = useState(0);
  const clear = () => { setProduct(null); setPickerKey(n => n + 1); };
  return <div className="space-y-5">
    {stockChecking && <ProductPicker key={pickerKey} onPick={setProduct} onClear={clear} />}
    <StoreFinder productId={product?.id} />
  </div>;
}
