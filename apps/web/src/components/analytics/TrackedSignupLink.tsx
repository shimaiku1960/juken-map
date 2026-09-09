import type { ComponentProps } from "react";
import { Link } from "react-router";
import { trackEvent } from "@/web/lib/analytics";

// 遷移先はこのコンポーネントが固定で決めるので、to も受け取らない。
// Next.js 版では href を Omit していた箇所（react-router では to が必須）。
type Props = Omit<ComponentProps<typeof Link>, "to" | "onClick"> & {
  location: "header" | "hero" | "middle" | "final" | "footer";
};

export default function TrackedSignupLink({ location, ...props }: Props) {
  return (
    <Link
      {...props}
      to="/signup"
      onClick={() =>
        trackEvent("signup_cta_click", {
          cta_location: location,
          destination: "signup",
        })
      }
    />
  );
}
