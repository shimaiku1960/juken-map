"use client";

import type { ComponentProps } from "react";
import Link from "next/link";
import { trackEvent } from "@/lib/analytics";

type Props = Omit<ComponentProps<typeof Link>, "href" | "onClick"> & {
  location: "header" | "hero" | "middle" | "final" | "footer";
};

export default function TrackedSignupLink({ location, ...props }: Props) {
  return (
    <Link
      {...props}
      href="/signup"
      onClick={() =>
        trackEvent("signup_cta_click", {
          cta_location: location,
          destination: "signup",
        })
      }
    />
  );
}
