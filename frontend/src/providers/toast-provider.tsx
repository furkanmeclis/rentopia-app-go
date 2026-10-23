"use client";

import { useEffect, type ReactNode } from "react";
import { toast } from "sonner";

import { Toaster } from "@/components/ui/sonner";

import { ApiError, setGlobalApiErrorHandler } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/**
 * 403 codes with their own text instead of the generic "forbidden" one:
 * support access (TEC-678), e.g. impersonating a member of a company that
 * has not opened support access.
 */
const FORBIDDEN_CODE_KEYS: Record<string, string> = {
  SUPPORT_GRANT_REQUIRED: "errors.codes.SUPPORT_GRANT_REQUIRED",
  SUPPORT_READ_ONLY: "errors.codes.SUPPORT_READ_ONLY",
};

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
        const key = FORBIDDEN_CODE_KEYS[error.code];
        showError(t(key ?? "common.error_forbidden"));
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
