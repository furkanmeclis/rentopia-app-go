import type {
  Customer,
  CustomerConsentRequest,
  CustomerConsents,
  CustomerContact,
  CustomerContactCreateRequest,
  CustomerContactPatchRequest,
  CustomerCreateRequest,
  CustomerDocument,
  CustomerDocumentCreateRequest,
  CustomerDriver,
  CustomerDriverCreateRequest,
  CustomerDriverPatchRequest,
  CustomerFacets,
  CustomerPage,
  CustomerPatchRequest,
  CustomerRevealed,
  DriverEligibility,
  ListCustomersParams,
  ListResponse,
} from "@/features/customers/types";
import type { FleetActivityPage } from "@/features/fleet/types";
import type { ResourceMeta } from "@/features/io/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/customers";
const customer = (uuid: string) => `${BASE}/${uuid}`;

type PageParams = { limit: number; offset: number };

/**
 * Customers of one organization (`X-Organization: {slug}`). Every read
 * answers masked identity / contact values; only `reveal` returns
 * plaintext (permission + reason, recorded by the API).
 */
export const customersService = {
  list(slug: string, params: ListCustomersParams) {
    return tenantRequest<CustomerPage>(slug, "GET", BASE, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<CustomerFacets>(slug, "GET", `${BASE}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Customer>(slug, "GET", customer(uuid));
  },
  create(slug: string, body: CustomerCreateRequest) {
    return tenantRequest<Customer>(slug, "POST", BASE, { body });
  },
  update(slug: string, uuid: string, body: CustomerPatchRequest) {
    return tenantRequest<Customer>(slug, "PATCH", customer(uuid), { body });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", customer(uuid));
  },
  reveal(slug: string, uuid: string, reason: string) {
    return tenantRequest<CustomerRevealed>(
      slug,
      "POST",
      `${customer(uuid)}/reveal`,
      { body: { reason } },
    );
  },
  anonymize(slug: string, uuid: string, reason: string) {
    return tenantRequest<Customer>(
      slug,
      "POST",
      `${customer(uuid)}/anonymize`,
      { body: { reason } },
    );
  },
  activity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<FleetActivityPage>(
      slug,
      "GET",
      `${customer(uuid)}/activity`,
      { query: params },
    );
  },
  contacts(slug: string, uuid: string) {
    return tenantRequest<ListResponse<CustomerContact>>(
      slug,
      "GET",
      `${customer(uuid)}/contacts`,
    );
  },
  createContact(
    slug: string,
    uuid: string,
    body: CustomerContactCreateRequest,
  ) {
    return tenantRequest<CustomerContact>(
      slug,
      "POST",
      `${customer(uuid)}/contacts`,
      { body },
    );
  },
  updateContact(
    slug: string,
    uuid: string,
    contactUuid: string,
    body: CustomerContactPatchRequest,
  ) {
    return tenantRequest<CustomerContact>(
      slug,
      "PATCH",
      `${customer(uuid)}/contacts/${contactUuid}`,
      { body },
    );
  },
  deleteContact(slug: string, uuid: string, contactUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${customer(uuid)}/contacts/${contactUuid}`,
    );
  },
  consents(slug: string, uuid: string) {
    return tenantRequest<CustomerConsents>(
      slug,
      "GET",
      `${customer(uuid)}/consents`,
    );
  },
  recordConsent(slug: string, uuid: string, body: CustomerConsentRequest) {
    return tenantRequest<CustomerConsents>(
      slug,
      "POST",
      `${customer(uuid)}/consents`,
      { body },
    );
  },
  drivers(slug: string, uuid: string) {
    return tenantRequest<ListResponse<CustomerDriver>>(
      slug,
      "GET",
      `${customer(uuid)}/drivers`,
    );
  },
  createDriver(slug: string, uuid: string, body: CustomerDriverCreateRequest) {
    return tenantRequest<CustomerDriver>(
      slug,
      "POST",
      `${customer(uuid)}/drivers`,
      { body },
    );
  },
  updateDriver(
    slug: string,
    uuid: string,
    driverUuid: string,
    body: CustomerDriverPatchRequest,
  ) {
    return tenantRequest<CustomerDriver>(
      slug,
      "PATCH",
      `${customer(uuid)}/drivers/${driverUuid}`,
      { body },
    );
  },
  deleteDriver(slug: string, uuid: string, driverUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${customer(uuid)}/drivers/${driverUuid}`,
    );
  },
  eligibility(
    slug: string,
    uuid: string,
    driverUuid: string,
    query: { min_driver_age?: number; min_license_years?: number },
  ) {
    return tenantRequest<DriverEligibility>(
      slug,
      "GET",
      `${customer(uuid)}/drivers/${driverUuid}/eligibility`,
      { query },
    );
  },
  documents(slug: string, uuid: string) {
    return tenantRequest<ListResponse<CustomerDocument>>(
      slug,
      "GET",
      `${customer(uuid)}/documents`,
    );
  },
  createDocument(
    slug: string,
    uuid: string,
    body: CustomerDocumentCreateRequest,
  ) {
    return tenantRequest<CustomerDocument>(
      slug,
      "POST",
      `${customer(uuid)}/documents`,
      { body },
    );
  },
  deleteDocument(slug: string, uuid: string, docUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${customer(uuid)}/documents/${docUuid}`,
    );
  },
  uploadDocumentFile(slug: string, uuid: string, docUuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<CustomerDocument>(
      slug,
      "PUT",
      `${customer(uuid)}/documents/${docUuid}/file`,
      form,
    );
  },
  downloadDocumentFile(slug: string, uuid: string, docUuid: string) {
    return tenantDownloadFile(
      slug,
      `${customer(uuid)}/documents/${docUuid}/file`,
    );
  },
};
