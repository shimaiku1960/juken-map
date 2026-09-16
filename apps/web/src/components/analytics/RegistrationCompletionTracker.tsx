import { useEffect } from "react";
import { reportRegistrationCompletion } from "@/web/lib/analytics";

export default function RegistrationCompletionTracker() {
  useEffect(() => {
    reportRegistrationCompletion();
  }, []);

  return null;
}
