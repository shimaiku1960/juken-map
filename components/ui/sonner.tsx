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
