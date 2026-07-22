import type { LucideIcon } from "lucide-react";
import { AlertCircle, CheckCircle2, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

const variants = {
  info: { icon: Info, className: "border-info/25 bg-info/10 text-info" },
  success: { icon: CheckCircle2, className: "border-success/25 bg-success/10 text-success" },
  warning: { icon: TriangleAlert, className: "border-warning/30 bg-warning/15 text-warning-foreground" },
  error: { icon: AlertCircle, className: "border-destructive/25 bg-destructive/10 text-destructive" },
} as const;

type InlineFeedbackProps = {
  variant?: keyof typeof variants;
  children: ReactNode;
  action?: ReactNode;
  icon?: LucideIcon;
  className?: string;
};

export default function InlineFeedback({ variant = "info", children, action, icon, className }: InlineFeedbackProps) {
  const selected = variants[variant];
  const Icon = icon ?? selected.icon;
  return (
    <div
      role={variant === "error" ? "alert" : "status"}
      className={cn("flex items-start gap-3 rounded-lg border px-4 py-3 text-sm", selected.className, className)}
    >
      <Icon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1">{children}</div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
