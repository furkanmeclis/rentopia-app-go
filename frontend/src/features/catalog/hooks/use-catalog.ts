"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { CATALOG_IMPORT_RUNNING } from "@/features/catalog/constants";
import {
  catalogService,
  type CatalogBrand,
  type CreateCatalogModelRequest,
  type ListBrandsParams,
  type ListImportsParams,
  type ListModelsParams,
  type ListTypesParams,
  type PatchCatalogBrandRequest,
  type PatchCatalogModelRequest,
  type PatchCatalogTypeRequest,
} from "@/features/catalog/services/catalog.service";
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
};

export function useCatalogMeta(
  resource: "brands" | "models" | "types" | "imports",
) {
  return useQuery({
    queryKey: catalogKeys.meta(resource),
    queryFn: () => {
      switch (resource) {
        case "brands":
          return catalogService.brandsMeta();
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
    void queryClient.invalidateQueries({ queryKey: catalogKeys.models() });
    void queryClient.invalidateQueries({ queryKey: catalogKeys.types() });
  };
}
