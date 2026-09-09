"use client";

import { toast } from "sonner";

export const notifyDemoReadOnly = () => {
  toast.info("デモアカウントは閲覧専用です", {
    description: "編集するには、ご自身のアカウントでログインしてください。",
  });
};
