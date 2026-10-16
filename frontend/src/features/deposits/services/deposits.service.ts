import type {
  DepositTotals,
  RentalDeposits,
  SecurityDeposit,
  SecurityDepositInput,
  SecurityDepositResolveInput,
} from "@/features/deposits/types";
import type { ListParams, Page } from "@/features/finance/types";
import type { ResourceMeta } from "@/features/io/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/finance/deposits";
const one = (uuid: string) => `${BASE}/${uuid}`;

/**
 * Security deposits of one organization (`/v1/tenant/finance/deposits`):
 * every call names the organization through `tenantRequest`. Actions whose
 * errors the dialogs render are `silent`.
 */
export const depositsService = {
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },
  list(slug: string, params: ListParams) {
    return tenantRequest<Page<SecurityDeposit>>(slug, "GET", BASE, {
      query: params,
    });
  },
  totals(slug: string, heldOverDays?: number) {
    return tenantRequest<DepositTotals>(slug, "GET", `${BASE}/totals`, {
      query: { held_over_days: heldOverDays },
    });
  },
  get(slug: string, uuid: string) {
    return tenantRequest<SecurityDeposit>(slug, "GET", one(uuid));
  },
  forRental(slug: string, rentalUuid: string) {
    return tenantRequest<RentalDeposits>(
      slug,
      "GET",
      `/v1/tenant/finance/rentals/${rentalUuid}/deposits`,
    );
  },
  take(slug: string, body: SecurityDepositInput) {
    return tenantRequest<SecurityDeposit>(slug, "POST", BASE, {
      body,
      silent: true,
    });
  },
  verify(slug: string, uuid: string) {
    return tenantRequest<SecurityDeposit>(slug, "POST", `${one(uuid)}/verify`, {
      silent: true,
    });
  },
  resolve(slug: string, uuid: string, body: SecurityDepositResolveInput) {
    return tenantRequest<SecurityDeposit>(
      slug,
      "POST",
      `${one(uuid)}/resolve`,
      { body, silent: true },
    );
  },
  cancel(slug: string, uuid: string, reason: string) {
    return tenantRequest<SecurityDeposit>(slug, "POST", `${one(uuid)}/cancel`, {
      body: { reason },
      silent: true,
    });
  },
  uploadDocument(slug: string, uuid: string, deduction: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<SecurityDeposit>(
      slug,
      "PUT",
      `${one(uuid)}/deductions/${deduction}/file`,
      form,
      { silent: true },
    );
  },
  downloadDocument(slug: string, uuid: string, deduction: string) {
    return tenantDownloadFile(
      slug,
      `${one(uuid)}/deductions/${deduction}/file`,
    );
  },
  removeDocument(slug: string, uuid: string, deduction: string) {
    return tenantRequest<SecurityDeposit>(
      slug,
      "DELETE",
      `${one(uuid)}/deductions/${deduction}/file`,
    );
  },
};
