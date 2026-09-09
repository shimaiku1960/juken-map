"use client"

import { useSyncExternalStore } from "react"
import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import { CircleCheckIcon, InfoIcon, TriangleAlertIcon, OctagonXIcon, Loader2Icon } from "lucide-react"

// トーストの出る位置はPCとスマホで変える。position は単一のプロップなので、
// メディアクエリを購読して出し分ける。ブレークポイントは Tailwind の md と揃える。
const DESKTOP_QUERY = "(min-width: 48rem)"

const subscribeToDesktop = (onStoreChange: () => void) => {
  const query = window.matchMedia(DESKTOP_QUERY)
  query.addEventListener("change", onStoreChange)
  return () => query.removeEventListener("change", onStoreChange)
}

const useIsDesktop = () =>
  useSyncExternalStore(
    subscribeToDesktop,
    () => window.matchMedia(DESKTOP_QUERY).matches,
    () => false
  )

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()
  const isDesktop = useIsDesktop()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      // PC=右上（sticky ヘッダー4remを避ける）。スマホ=下（ボトムナビを避ける
      // 分の持ち上げは globals.css 側で指定）。
      position={isDesktop ? "top-right" : "bottom-right"}
      offset={isDesktop ? { top: "5rem" } : undefined}
      className="toaster group"
      richColors
      icons={{
        success: (
          <CircleCheckIcon className="size-4" />
        ),
        info: (
          <InfoIcon className="size-4" />
        ),
        warning: (
          <TriangleAlertIcon className="size-4" />
        ),
        error: (
          <OctagonXIcon className="size-4" />
        ),
        loading: (
          <Loader2Icon className="size-4 animate-spin" />
        ),
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--success-bg": "color-mix(in oklch, var(--success) 12%, var(--popover))",
          "--success-border": "color-mix(in oklch, var(--success) 35%, var(--border))",
          "--success-text": "color-mix(in oklch, var(--success) 78%, var(--foreground))",
          "--info-bg": "color-mix(in oklch, var(--info) 10%, var(--popover))",
          "--info-border": "color-mix(in oklch, var(--info) 32%, var(--border))",
          "--info-text": "color-mix(in oklch, var(--info) 76%, var(--foreground))",
          "--warning-bg": "color-mix(in oklch, var(--warning) 13%, var(--popover))",
          "--warning-border": "color-mix(in oklch, var(--warning) 38%, var(--border))",
          "--warning-text": "color-mix(in oklch, var(--warning) 50%, var(--foreground))",
          "--error-bg": "color-mix(in oklch, var(--destructive) 10%, var(--popover))",
          "--error-border": "color-mix(in oklch, var(--destructive) 32%, var(--border))",
          "--error-text": "color-mix(in oklch, var(--destructive) 76%, var(--foreground))",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "cn-toast",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
