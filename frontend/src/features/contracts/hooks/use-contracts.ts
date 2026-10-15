"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { contractsService } from "@/features/contracts/services/contracts.service";
import type {
  ContractPreviewRequest,
  ContractTemplateRequest,
  DocumentApproveRequest,
  DocumentPrepareRequest,
  ListDocumentsParams,
  ListTemplatesParams,
} from "@/features/contracts/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const contractKeys = {
  templates: (slug: string) => ["tenant", slug, "contract-templates"] as const,
  templateList: (slug: string, params: Record<string, unknown>) =>
    [...contractKeys.templates(slug), "list", params] as const,
  templatesMeta: (slug: string) =>
    [...contractKeys.templates(slug), "meta"] as const,
  placeholders: (slug: string) =>
    [...contractKeys.templates(slug), "placeholders"] as const,
  template: (slug: string, uuid: string) =>
    [...contractKeys.templates(slug), "detail", uuid] as const,
  documents: (slug: string) => ["tenant", slug, "documents"] as const,
  documentList: (slug: string, params: Record<string, unknown>) =>
    [...contractKeys.documents(slug), "list", params] as const,
  document: (slug: string, uuid: string) =>
    [...contractKeys.documents(slug), "detail", uuid] as const,
  version: (slug: string, doc: string, ver: string) =>
    [...contractKeys.documents(slug), "version", doc, ver] as const,
};

// ------------------------------------------------------------ templates

export function useContractTemplates(params: ListTemplatesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.templateList(slug, params),
    queryFn: () => contractsService.templates(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useContractTemplatesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.templatesMeta(slug),
    queryFn: () => contractsService.templatesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function usePlaceholders() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.placeholders(slug),
    queryFn: () => contractsService.placeholders(slug),
    staleTime: 30 * 60_000,
  });
}

export function useContractTemplate(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.template(slug, uuid),
    queryFn: () => contractsService.template(slug, uuid),
    enabled: Boolean(uuid),
  });
}

function useInvalidateTemplates() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: contractKeys.templates(slug) });
}

export function useCreateTemplate() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTemplates();
  return useMutation({
    mutationFn: (body: ContractTemplateRequest) =>
      contractsService.createTemplate(slug, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("contracts.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateTemplate(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTemplates();
  return useMutation({
    mutationFn: (body: ContractTemplateRequest) =>
      contractsService.updateTemplate(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("contracts.toast.saved"));
      void invalidate();
    },
  });
}

export function useTemplateAction() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTemplates();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
    }: {
      uuid: string;
      action: "activate" | "new_version" | "delete";
    }) => {
      if (action === "activate") {
        return contractsService.activateTemplate(slug, uuid);
      }
      if (action === "new_version") {
        return contractsService.newVersion(slug, uuid);
      }
      return contractsService.deleteTemplate(slug, uuid);
    },
    onSuccess: (_, v) => {
      appToast.success(
        t(
          v.action === "activate"
            ? "contracts.toast.activated"
            : v.action === "new_version"
              ? "contracts.toast.new_version"
              : "contracts.toast.deleted",
        ),
      );
      void invalidate();
    },
  });
}

export function usePreview() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: ContractPreviewRequest) =>
      contractsService.preview(slug, body),
    meta: { silent: true },
  });
}

// ------------------------------------------------------------ documents

export function useDocuments(params: ListDocumentsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.documentList(slug, params),
    queryFn: () => contractsService.documents(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useDocument(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.document(slug, uuid ?? ""),
    queryFn: () => contractsService.document(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useDocumentVersion(doc: string | null, ver: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: contractKeys.version(slug, doc ?? "", ver ?? ""),
    queryFn: () => contractsService.version(slug, doc ?? "", ver ?? ""),
    enabled: Boolean(doc && ver),
    // Versions are immutable; the PDF status changes (worker).
    refetchInterval: (query) =>
      query.state.data?.pdf_status === "pending" ? 3000 : false,
  });
}

/** Documents change rentals (contract state) and reservations. */
function useInvalidateDocuments() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: contractKeys.documents(slug) }),
      queryClient.invalidateQueries({ queryKey: ["tenant", slug, "rentals"] }),
      queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "reservations"],
      }),
    ]);
}

export function usePrepareDocument() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateDocuments();
  return useMutation({
    mutationFn: (body: DocumentPrepareRequest) =>
      contractsService.prepare(slug, body),
    onSuccess: (out) => {
      appToast.success(
        t(
          out.created
            ? "contracts.toast.version_created"
            : "contracts.toast.up_to_date",
        ),
      );
      void invalidate();
    },
  });
}

export function useRefreshDocument() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateDocuments();
  return useMutation({
    mutationFn: (uuid: string) => contractsService.refresh(slug, uuid),
    onSuccess: (out) => {
      appToast.success(
        t(
          out.created
            ? "contracts.toast.version_created"
            : "contracts.toast.up_to_date",
        ),
      );
      void invalidate();
    },
  });
}

export function useIssueVersion() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateDocuments();
  return useMutation({
    mutationFn: ({ doc, ver }: { doc: string; ver: string }) =>
      contractsService.issue(slug, doc, ver),
    onSuccess: () => {
      appToast.success(t("contracts.toast.issued"));
      void invalidate();
    },
  });
}

export function useApproveVersion() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateDocuments();
  return useMutation({
    mutationFn: ({
      doc,
      ver,
      body,
    }: {
      doc: string;
      ver: string;
      body: DocumentApproveRequest;
    }) => contractsService.approve(slug, doc, ver, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("contracts.toast.approved"));
      void invalidate();
    },
  });
}

export function useDownloadPdf() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      doc,
      ver,
      fallbackName,
    }: {
      doc: string;
      ver: string;
      fallbackName: string;
    }) => {
      const { blob, filename } = await contractsService.downloadPdf(
        slug,
        doc,
        ver,
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
      // Not rendered yet: the API queued it; the version polls its status.
      if (error instanceof ApiError && error.code === "PDF_NOT_READY") {
        appToast.info(t("contracts.toast.pdf_pending"));
        void queryClient.invalidateQueries({
          queryKey: contractKeys.documents(slug),
        });
        return;
      }
      appToast.error(t("contracts.toast.download_failed"));
    },
  });
}
