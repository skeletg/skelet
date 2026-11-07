import { SparklesIcon } from "lucide-react";

// The assistant's mark, in the store's accent
export function AssistantSpark({ className = "" }: { className?: string }) {
  return <SparklesIcon strokeWidth={1.6} aria-hidden="true" className={`text-signal ${className}`} />;
}
