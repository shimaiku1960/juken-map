import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { Toaster } from "@/web/components/ui/sonner";
import { queryClient } from "@/web/lib/query-client";
import App from "@/web/App";
import "@/web/index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Toaster />
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>
);
