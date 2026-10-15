"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { contractKeys } from "@/features/contracts/hooks/use-contracts";
import { contractsService } from "@/features/contracts/services/contracts.service";
import type { ApprovalLinkCreateRequest } from "@/features/contracts/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Remote approval links of a document (TEC-711). */
const linkKeys = {
  links: (slug: string, doc: string) =>
    [...contractKeys.documents(slug), "approval-links", doc] as const,
  signers: (slug: string, doc: string) =>
    [...contractKeys.documents(slug), "signers", doc] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: contractKeys.documents(slug) });
}

export function useDocumentSigners(doc: string | null, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: linkKeys.signers(slug, doc ?? ""),
    queryFn: () => contractsService.signers(slug, doc ?? ""),
    enabled: Boolean(doc) && enabled,
  });
}

export function useApprovalLinks(doc: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: linkKeys.links(slug, doc ?? ""),
    queryFn: () => contractsService.approvalLinks(slug, doc ?? ""),
    enabled: Boolean(doc),
  });
}

export function useCreateApprovalLink() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      doc,
      ver,
      body,
    }: {
      doc: string;
      ver: string;
      body: ApprovalLinkCreateRequest;
    }) => contractsService.createApprovalLink(slug, doc, ver, body),
    meta: { silent: true },
    onSuccess: (out) => {
      if (out.sms_error) {
        appToast.warning(t("contracts.remote.toast.created_sms_failed"));
      } else {
        appToast.success(
          t(
            out.sms
              ? "contracts.remote.toast.created_sent"
              : "contracts.remote.toast.created",
          ),
        );
      }
      void invalidate();
    },
  });
}

export function useSendApprovalLink() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ doc, link }: { doc: string; link: string }) =>
      contractsService.sendApprovalLink(slug, doc, link),
    onSuccess: (out) => {
      appToast.success(
        t("contracts.remote.toast.sent", { phone: out.phone_masked }),
      );
      void invalidate();
    },
  });
}

export function useRevokeApprovalLink() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ doc, link }: { doc: string; link: string }) =>
      contractsService.revokeApprovalLink(slug, doc, link),
    onSuccess: () => {
      appToast.success(t("contracts.remote.toast.revoked"));
      void invalidate();
    },
  });
}

export function useDownloadApprovalPdf() {
  const { slug } = useTenant();
  const { t } = useLocale();
  return useMutation({
    mutationFn: async ({
      doc,
      approval,
      fallbackName,
    }: {
      doc: string;
      approval: string;
      fallbackName: string;
    }) => {
      const { blob, filename } = await contractsService.downloadApprovalPdf(
        slug,
        doc,
        approval,
      );
      const href = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = href;
      link.download = filename ?? fallbackName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(href);
    },
    onError: (error) => {
      appToast.error(
        error instanceof ApiError && error.code === "PDF_NOT_READY"
          ? t("contracts.toast.pdf_pending")
          : t("contracts.toast.download_failed"),
      );
    },
  });
}
