"use client";

import { useEffect, type ReactNode } from "react";
import { toast } from "sonner";

import { Toaster } from "@/components/ui/sonner";

import { ApiError, setGlobalApiErrorHandler } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export function ToastProvider({ children }: { children: ReactNode }) {
  const { t } = useLocale();

  useEffect(() => {
    // Same message, same id: parallel failing requests show one toast.
    const showError = (message: string) =>
      toast.error(message, { id: message });

    setGlobalApiErrorHandler((error: ApiError) => {
      if (error.isUnauthorized) {
        showError(t("auth.session_expired"));
        return;
      }
      if (error.isForbidden) {
        showError(t("common.error_forbidden"));
        return;
      }
      if (error.isNotFound) {
        showError(t("common.error_not_found"));
        return;
      }
      if (error.isValidation) {
        showError(error.message || t("common.error_validation"));
        return;
      }
      if (error.isServer) {
        showError(
          error.requestId
            ? `${t("common.error_server")} (${error.requestId})`
            : t("common.error_server"),
        );
        return;
      }
      showError(error.message || t("common.error_generic"));
    });

    return () => setGlobalApiErrorHandler(null);
  }, [t]);

  return (
    <>
      {children}
      <Toaster position="top-right" closeButton />
    </>
  );
}

export const appToast = {
  success: (message: string) => toast.success(message),
  error: (message: string) => toast.error(message),
  warning: (message: string) => toast.warning(message),
  info: (message: string) => toast.info(message),
};
