import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export default function PageShell({ className, ...props }: ComponentProps<"main">) {
  return (
    <main
      className={cn("mx-auto w-full max-w-3xl px-4 py-6 sm:px-8 sm:py-8", className)}
      {...props}
    />
  );
}
