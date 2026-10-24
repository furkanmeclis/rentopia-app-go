"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { CATALOG_IMPORT_RUNNING } from "@/features/catalog/constants";
import {
  catalogService,
  type CatalogBrand,
  type CatalogDisplayBrand,
  type CreateCatalogModelRequest,
  type ListBrandsParams,
  type ListDisplayBrandsParams,
  type ListImportsParams,
  type ListModelsParams,
  type ListTypesParams,
  type PatchCatalogBrandRequest,
  type PatchCatalogDisplayBrandRequest,
  type UnmergeCatalogDisplayBrandRequest,
  type PatchCatalogModelRequest,
  type PatchCatalogTypeRequest,
  type MotomarksLinkMode,
  type PatchBrandLogoSettingsRequest,
} from "@/features/catalog/services/catalog.service";
import type { ServerListParams } from "@/components/entity";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const catalogKeys = {
  all: ["catalog"] as const,
  meta: (resource: string) => ["catalog", "meta", resource] as const,
  brands: () => ["catalog", "brands"] as const,
  brandList: (params: Record<string, unknown>) =>
    ["catalog", "brands", "list", params] as const,
  brandOptions: () => ["catalog", "brands", "options"] as const,
  displayBrands: () => ["catalog", "display-brands"] as const,
  displayBrandList: (params: Record<string, unknown>) =>
    ["catalog", "display-brands", "list", params] as const,
  displayBrandOptions: () => ["catalog", "display-brands", "options"] as const,
  displayBrand: (uuid: string) =>
    ["catalog", "display-brands", "detail", uuid] as const,
  models: () => ["catalog", "models"] as const,
  modelList: (params: Record<string, unknown>) =>
    ["catalog", "models", "list", params] as const,
  types: () => ["catalog", "types"] as const,
  typeList: (params: Record<string, unknown>) =>
    ["catalog", "types", "list", params] as const,
  type: (uuid: string) => ["catalog", "types", "detail", uuid] as const,
  imports: () => ["catalog", "imports"] as const,
  importList: (params: Record<string, unknown>) =>
    ["catalog", "imports", "list", params] as const,
  import: (uuid: string) => ["catalog", "imports", "detail", uuid] as const,
  logoSettings: () => ["catalog", "brand-logos", "settings"] as const,
  logoRuns: (params: Record<string, unknown>) =>
    ["catalog", "brand-logos", "runs", params] as const,
  logoUnmatched: (params: Record<string, unknown>) =>
    ["catalog", "brand-logos", "unmatched", params] as const,
};

export function useCatalogMeta(
  resource: "brands" | "display-brands" | "models" | "types" | "imports",
) {
  return useQuery({
    queryKey: catalogKeys.meta(resource),
    queryFn: () => {
      switch (resource) {
        case "brands":
          return catalogService.brandsMeta();
        case "display-brands":
          return catalogService.displayBrandsMeta();
        case "models":
          return catalogService.modelsMeta();
        case "types":
          return catalogService.typesMeta();
        default:
          return catalogService.importsMeta();
      }
    },
    staleTime: 5 * 60_000,
  });
}

export function useCatalogBrands(params: ListBrandsParams) {
  return useQuery({
    queryKey: catalogKeys.brandList(params),
    queryFn: () => catalogService.listBrands(params),
    placeholderData: (prev) => prev,
  });
}

/** Every brand (two or so pages of 100) for facet options and pickers. */
export function useCatalogBrandOptions() {
  return useQuery({
    queryKey: catalogKeys.brandOptions(),
    queryFn: async () => {
      const all: CatalogBrand[] = [];
      for (let offset = 0; offset < 5000; offset += 100) {
        const page = await catalogService.listBrands({
          limit: 100,
          offset,
          sort: "display_name",
        });
        all.push(...page.items);
        if (all.length >= page.total || page.items.length === 0) break;
      }
      return all;
    },
    staleTime: 5 * 60_000,
  });
}

export function useCatalogDisplayBrands(params: ListDisplayBrandsParams) {
  return useQuery({
    queryKey: catalogKeys.displayBrandList(params),
    queryFn: () => catalogService.listDisplayBrands(params),
    placeholderData: (prev) => prev,
  });
}

export function useCatalogDisplayBrand(uuid: string | undefined) {
  return useQuery({
    queryKey: catalogKeys.displayBrand(uuid ?? ""),
    queryFn: () => catalogService.getDisplayBrand(uuid!),
    enabled: Boolean(uuid),
  });
}

/** Every display brand (two or so pages of 100) for facets and pickers. */
export function useCatalogDisplayBrandOptions() {
  return useQuery({
    queryKey: catalogKeys.displayBrandOptions(),
    queryFn: async () => {
      const all: CatalogDisplayBrand[] = [];
      for (let offset = 0; offset < 5000; offset += 100) {
        const page = await catalogService.listDisplayBrands({
          limit: 100,
          offset,
          sort: "name",
        });
        all.push(...page.items);
        if (all.length >= page.total || page.items.length === 0) break;
      }
      return all;
    },
    staleTime: 5 * 60_000,
  });
}

function useInvalidateBrands() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
    void queryClient.invalidateQueries({
      queryKey: catalogKeys.displayBrands(),
    });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
  };
}

export function usePatchDisplayBrand() {
  const invalidate = useInvalidateBrands();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; body: PatchCatalogDisplayBrandRequest }) =>
      catalogService.patchDisplayBrand(v.uuid, v.body),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.display_brand_saved"));
    },
  });
}

export function useMergeDisplayBrand() {
  const invalidate = useInvalidateBrands();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; intoUuid: string }) =>
      catalogService.mergeDisplayBrand(v.uuid, v.intoUuid),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.display_brand_merged"));
    },
  });
}

export function useUnmergeDisplayBrand() {
  const invalidate = useInvalidateBrands();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: {
      uuid: string;
      body: UnmergeCatalogDisplayBrandRequest;
    }) => catalogService.unmergeDisplayBrand(v.uuid, v.body),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.display_brand_unmerged"));
    },
  });
}

export function useDisplayBrandLogo() {
  const invalidate = useInvalidateBrands();
  const { t } = useLocale();
  const upload = useAppMutation({
    mutationFn: (v: { uuid: string; file: File }) =>
      catalogService.uploadDisplayBrandLogo(v.uuid, v.file),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.logo_saved"));
    },
  });
  const clear = useAppMutation({
    mutationFn: (uuid: string) => catalogService.clearDisplayBrandLogo(uuid),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.logo_removed"));
    },
  });
  return { upload, clear };
}

export function useBulkModelRules() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (modelUuids: string[]) =>
      catalogService.bulkModelRules(modelUuids),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
      appToast.success(
        t("catalog.toast.rules_saved", {
          rules: res.rules_saved,
          types: res.types_assigned,
        }),
      );
    },
  });
}

export function useCatalogModels(params: ListModelsParams) {
  return useQuery({
    queryKey: catalogKeys.modelList(params),
    queryFn: () => catalogService.listModels(params),
    placeholderData: (prev) => prev,
  });
}

export function useCatalogTypes(params: ListTypesParams) {
  return useQuery({
    queryKey: catalogKeys.typeList(params),
    queryFn: () => catalogService.listTypes(params),
    placeholderData: (prev) => prev,
  });
}

export function useCatalogType(uuid: string) {
  return useQuery({
    queryKey: catalogKeys.type(uuid),
    queryFn: () => catalogService.getType(uuid),
  });
}

export function useCatalogImports(params: ListImportsParams) {
  return useQuery({
    queryKey: catalogKeys.importList(params),
    queryFn: () => catalogService.listImports(params),
    placeholderData: (prev) => prev,
    refetchInterval: (query) =>
      query.state.data?.items.some((i) =>
        CATALOG_IMPORT_RUNNING.includes(i.status),
      )
        ? 3000
        : false,
  });
}

/** Polls every 2s while the dry run or apply is running. */
export function useCatalogImport(uuid: string) {
  return useQuery({
    queryKey: catalogKeys.import(uuid),
    queryFn: () => catalogService.getImport(uuid),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && CATALOG_IMPORT_RUNNING.includes(status) ? 2000 : false;
    },
  });
}

export function usePatchBrand() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; body: PatchCatalogBrandRequest }) =>
      catalogService.patchBrand(v.uuid, v.body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
      void queryClient.invalidateQueries({
        queryKey: catalogKeys.displayBrands(),
      });
      appToast.success(t("catalog.toast.brand_saved"));
    },
  });
}

export function useCreateModel() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (body: CreateCatalogModelRequest) =>
      catalogService.createModel(body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
      appToast.success(t("catalog.toast.model_created"));
    },
  });
}

export function usePatchModel() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; body: PatchCatalogModelRequest }) =>
      catalogService.patchModel(v.uuid, v.body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
      appToast.success(t("catalog.toast.model_saved"));
    },
  });
}

export function useMergeModel() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; intoUuid: string }) =>
      catalogService.mergeModel(v.uuid, v.intoUuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
      appToast.success(t("catalog.toast.model_merged"));
    },
  });
}

export function usePatchType() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: { uuid: string; body: PatchCatalogTypeRequest }) =>
      catalogService.patchType(v.uuid, v.body),
    onSuccess: (detail) => {
      queryClient.setQueryData(catalogKeys.type(detail.uuid), detail);
      void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
      void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
      appToast.success(t("catalog.toast.type_saved"));
    },
  });
}

export function useUploadImport() {
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (v: { file: File; period?: string }) =>
      catalogService.uploadImport(v.file, v.period),
    onSuccess: (job) => {
      queryClient.setQueryData(catalogKeys.import(job.uuid), job);
      void queryClient.invalidateQueries({ queryKey: catalogKeys.imports() });
    },
  });
}

export function useApplyImport() {
  const queryClient = useQueryClient();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (uuid: string) => catalogService.applyImport(uuid),
    onSuccess: (job) => {
      queryClient.setQueryData(catalogKeys.import(job.uuid), job);
      void queryClient.invalidateQueries({ queryKey: catalogKeys.imports() });
      appToast.success(t("catalog.toast.apply_queued"));
    },
  });
}

/** After an apply finishes, lists elsewhere are stale. */
export function useInvalidateCatalog() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
    void queryClient.invalidateQueries({
      queryKey: catalogKeys.displayBrands(),
    });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
  };
}

// ---------- brand logo sync (motomarks-mirror) ----------

export function useBrandLogoSettings(options?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: catalogKeys.logoSettings(),
    queryFn: () => catalogService.brandLogoSettings(),
    refetchInterval: options?.refetchInterval,
  });
}

function useInvalidateBrandLogos() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({
      queryKey: catalogKeys.logoSettings(),
    });
    void queryClient.invalidateQueries({
      queryKey: ["catalog", "brand-logos", "runs"],
    });
    void queryClient.invalidateQueries({
      queryKey: ["catalog", "brand-logos", "unmatched"],
    });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.brands() });
    void queryClient.invalidateQueries({
      queryKey: catalogKeys.displayBrands(),
    });
  };
}

export function usePatchBrandLogoSettings() {
  const invalidate = useInvalidateBrandLogos();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (body: PatchBrandLogoSettingsRequest) =>
      catalogService.patchBrandLogoSettings(body),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.logo_settings_saved"));
    },
  });
}

export function useTestBrandLogoConnection() {
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: () => catalogService.testBrandLogoConnection(),
    onSuccess: () => appToast.success(t("catalog.logos.test_ok")),
  });
}

export function useRequestBrandLogoSync() {
  const invalidate = useInvalidateBrandLogos();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: () => catalogService.requestBrandLogoSync(),
    onSuccess: () => {
      invalidate();
      appToast.success(t("catalog.toast.logo_sync_requested"));
    },
  });
}

export function useBrandLogoRuns(
  params: ServerListParams,
  options?: { refetchInterval?: number },
) {
  return useQuery({
    queryKey: catalogKeys.logoRuns(params),
    queryFn: () => catalogService.listBrandLogoRuns(params),
    placeholderData: (prev) => prev,
    refetchInterval: options?.refetchInterval,
  });
}

export function useUnmatchedDisplayBrands(params: ServerListParams) {
  return useQuery({
    queryKey: catalogKeys.logoUnmatched(params),
    queryFn: () => catalogService.listUnmatchedDisplayBrands(params),
    placeholderData: (prev) => prev,
  });
}

export function useSetDisplayBrandMotomarks() {
  const invalidate = useInvalidateBrandLogos();
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: (v: {
      uuid: string;
      mode: MotomarksLinkMode;
      motomarksId?: string;
    }) =>
      catalogService.setDisplayBrandMotomarks(v.uuid, v.mode, v.motomarksId),
    onSuccess: (_res, v) => {
      invalidate();
      appToast.success(
        t(
          v.mode === "link"
            ? "catalog.toast.logo_linked"
            : v.mode === "skip"
              ? "catalog.toast.logo_skipped"
              : "catalog.toast.logo_unlinked",
        ),
      );
    },
  });
}
