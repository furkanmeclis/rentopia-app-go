import type { ServerListParams } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type CatalogBrand = Schemas["CatalogBrand"];
export type CatalogDisplayBrand = Schemas["CatalogDisplayBrand"];
export type CatalogDisplayBrandMember = Schemas["CatalogDisplayBrandMember"];
export type PatchCatalogDisplayBrandRequest =
  Schemas["PatchCatalogDisplayBrandRequest"];
export type UnmergeCatalogDisplayBrandRequest =
  Schemas["UnmergeCatalogDisplayBrandRequest"];
export type CatalogBulkRulesResult = Schemas["CatalogBulkRulesResult"];
export type CatalogModel = Schemas["CatalogModel"];
export type CatalogType = Schemas["CatalogType"];
export type CatalogTypeDetail = Schemas["CatalogTypeDetail"];
export type CatalogImport = Schemas["CatalogImport"];
export type CatalogImportCounts = Schemas["CatalogImportCounts"];
export type CatalogImportDiff = Schemas["CatalogImportDiff"];
export type CatalogBrandCategory = Schemas["CatalogBrandCategory"];
export type CatalogFuel = Schemas["CatalogFuel"];
export type CatalogTransmission = Schemas["CatalogTransmission"];
export type CatalogModelSource = Schemas["CatalogModelSource"];
export type CatalogImportStatus = Schemas["CatalogImportStatus"];
export type PatchCatalogBrandRequest = Schemas["PatchCatalogBrandRequest"];
export type PatchCatalogModelRequest = Schemas["PatchCatalogModelRequest"];
export type PatchCatalogTypeRequest = Schemas["PatchCatalogTypeRequest"];
export type CreateCatalogModelRequest = Schemas["CreateCatalogModelRequest"];
export type ResourceMeta = Schemas["ResourceMeta"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type ListBrandsParams = ServerListParams & {
  category?: string;
  is_active?: string;
  display_brand?: string;
};

export type ListDisplayBrandsParams = ServerListParams & {
  is_active?: string;
  merged?: string;
};

export type ListModelsParams = ServerListParams & {
  brand?: string;
  display_brand?: string;
  source?: string;
};

export type ListTypesParams = ServerListParams & {
  brand?: string;
  display_brand?: string;
  model?: string;
  category?: string;
  year?: number;
  unresolved?: string;
  is_active?: string;
  fuel?: string;
  transmission?: string;
};

export type ListImportsParams = ServerListParams & {
  status?: string;
};

const base = "/v1/platform/catalog";

export const catalogService = {
  brandsMeta() {
    return platformRequest<ResourceMeta>("GET", `${base}/brands/meta`);
  },
  modelsMeta() {
    return platformRequest<ResourceMeta>("GET", `${base}/models/meta`);
  },
  typesMeta() {
    return platformRequest<ResourceMeta>("GET", `${base}/types/meta`);
  },
  importsMeta() {
    return platformRequest<ResourceMeta>("GET", `${base}/imports/meta`);
  },
  displayBrandsMeta() {
    return platformRequest<ResourceMeta>("GET", `${base}/display-brands/meta`);
  },

  listDisplayBrands(params: ListDisplayBrandsParams) {
    return platformRequest<Page<CatalogDisplayBrand>>(
      "GET",
      `${base}/display-brands`,
      { query: { ...params } },
    );
  },
  getDisplayBrand(uuid: string) {
    return platformRequest<CatalogDisplayBrand>(
      "GET",
      `${base}/display-brands/${uuid}`,
    );
  },
  patchDisplayBrand(uuid: string, body: PatchCatalogDisplayBrandRequest) {
    return platformRequest<CatalogDisplayBrand>(
      "PATCH",
      `${base}/display-brands/${uuid}`,
      { body },
    );
  },
  mergeDisplayBrand(uuid: string, intoUuid: string) {
    return platformRequest<CatalogDisplayBrand>(
      "POST",
      `${base}/display-brands/${uuid}/merge`,
      { body: { into_uuid: intoUuid } },
    );
  },
  unmergeDisplayBrand(uuid: string, body: UnmergeCatalogDisplayBrandRequest) {
    return platformRequest<CatalogDisplayBrand>(
      "POST",
      `${base}/display-brands/${uuid}/unmerge`,
      { body },
    );
  },
  uploadDisplayBrandLogo(uuid: string, file: File) {
    const form = new FormData();
    form.append("logo", file);
    return platformFormRequest<CatalogDisplayBrand>(
      "PUT",
      `${base}/display-brands/${uuid}/logo`,
      form,
    );
  },
  clearDisplayBrandLogo(uuid: string) {
    return platformRequest<CatalogDisplayBrand>(
      "DELETE",
      `${base}/display-brands/${uuid}/logo`,
    );
  },

  listBrands(params: ListBrandsParams) {
    return platformRequest<Page<CatalogBrand>>("GET", `${base}/brands`, {
      query: { ...params },
    });
  },
  getBrand(uuid: string) {
    return platformRequest<CatalogBrand>("GET", `${base}/brands/${uuid}`);
  },
  patchBrand(uuid: string, body: PatchCatalogBrandRequest) {
    return platformRequest<CatalogBrand>("PATCH", `${base}/brands/${uuid}`, {
      body,
    });
  },

  listModels(params: ListModelsParams) {
    return platformRequest<Page<CatalogModel>>("GET", `${base}/models`, {
      query: { ...params },
    });
  },
  getModel(uuid: string) {
    return platformRequest<CatalogModel>("GET", `${base}/models/${uuid}`);
  },
  createModel(body: CreateCatalogModelRequest) {
    return platformRequest<CatalogModel>("POST", `${base}/models`, { body });
  },
  patchModel(uuid: string, body: PatchCatalogModelRequest) {
    return platformRequest<CatalogModel>("PATCH", `${base}/models/${uuid}`, {
      body,
    });
  },
  bulkModelRules(modelUuids: string[]) {
    return platformRequest<CatalogBulkRulesResult>(
      "POST",
      `${base}/models/bulk-rules`,
      { body: { model_uuids: modelUuids } },
    );
  },
  mergeModel(uuid: string, intoUuid: string) {
    return platformRequest<CatalogModel>(
      "POST",
      `${base}/models/${uuid}/merge`,
      { body: { into_uuid: intoUuid } },
    );
  },

  listTypes(params: ListTypesParams) {
    return platformRequest<Page<CatalogType>>("GET", `${base}/types`, {
      query: { ...params },
    });
  },
  getType(uuid: string) {
    return platformRequest<CatalogTypeDetail>("GET", `${base}/types/${uuid}`);
  },
  patchType(uuid: string, body: PatchCatalogTypeRequest) {
    return platformRequest<CatalogTypeDetail>(
      "PATCH",
      `${base}/types/${uuid}`,
      { body },
    );
  },

  listImports(params: ListImportsParams) {
    return platformRequest<Page<CatalogImport>>("GET", `${base}/imports`, {
      query: { ...params },
    });
  },
  getImport(uuid: string) {
    return platformRequest<CatalogImport>("GET", `${base}/imports/${uuid}`);
  },
  uploadImport(file: File, period?: string) {
    const form = new FormData();
    form.append("file", file);
    if (period) form.append("period", period);
    return platformFormRequest<CatalogImport>("POST", `${base}/imports`, form);
  },
  applyImport(uuid: string) {
    return platformRequest<CatalogImport>(
      "POST",
      `${base}/imports/${uuid}/apply`,
    );
  },
};
