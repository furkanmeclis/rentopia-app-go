import type {
  Agreement,
  AgreementBillingPlan,
  AgreementContactRequest,
  AgreementDriverRequest,
  AgreementFacets,
  AgreementInput,
  AgreementPage,
  AgreementRentalRow,
  AgreementStatement,
  AgreementStatusRequest,
  AgreementTariff,
  ListAgreementsParams,
} from "@/features/agreements/types";
import type { ResourceMeta } from "@/features/io/types";
import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type ActivityPage = components["schemas"]["EnvelopeFleetActivityPage"]["data"];

const AGREEMENTS = "/v1/tenant/agreements";
const one = (uuid: string) => `${AGREEMENTS}/${uuid}`;

/** Corporate agreements (`tenantRequest` only). */
export const agreementsService = {
  list(slug: string, params: ListAgreementsParams) {
    return tenantRequest<AgreementPage>(slug, "GET", AGREEMENTS, {
      query: params,
    });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${AGREEMENTS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<AgreementFacets>(slug, "GET", `${AGREEMENTS}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Agreement>(slug, "GET", one(uuid));
  },
  create(slug: string, body: AgreementInput) {
    return tenantRequest<Agreement>(slug, "POST", AGREEMENTS, { body });
  },
  update(slug: string, uuid: string, body: AgreementInput) {
    return tenantRequest<Agreement>(slug, "PATCH", one(uuid), { body });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", one(uuid));
  },
  setStatus(slug: string, uuid: string, body: AgreementStatusRequest) {
    return tenantRequest<Agreement>(slug, "POST", `${one(uuid)}/status`, {
      body,
    });
  },
  addContact(slug: string, uuid: string, body: AgreementContactRequest) {
    return tenantRequest<Agreement>(slug, "POST", `${one(uuid)}/contacts`, {
      body,
    });
  },
  removeContact(slug: string, uuid: string, contact: string) {
    return tenantRequest<Agreement>(
      slug,
      "DELETE",
      `${one(uuid)}/contacts/${contact}`,
    );
  },
  addDriver(slug: string, uuid: string, body: AgreementDriverRequest) {
    return tenantRequest<Agreement>(slug, "POST", `${one(uuid)}/drivers`, {
      body,
    });
  },
  removeDriver(slug: string, uuid: string, driver: string) {
    return tenantRequest<Agreement>(
      slug,
      "DELETE",
      `${one(uuid)}/drivers/${driver}`,
    );
  },
  tariffs(slug: string, uuid: string) {
    return tenantRequest<{ items: AgreementTariff[] }>(
      slug,
      "GET",
      `${one(uuid)}/tariffs`,
    );
  },
  rentals(slug: string, uuid: string) {
    return tenantRequest<{ items: AgreementRentalRow[] }>(
      slug,
      "GET",
      `${one(uuid)}/rentals`,
    );
  },
  billing(slug: string, uuid: string, rental?: string) {
    return tenantRequest<AgreementBillingPlan>(
      slug,
      "GET",
      `${one(uuid)}/billing`,
      { query: rental ? { rental } : undefined },
    );
  },
  statement(slug: string, uuid: string) {
    return tenantRequest<AgreementStatement>(
      slug,
      "GET",
      `${one(uuid)}/statement`,
    );
  },
  activity(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ActivityPage>(slug, "GET", `${one(uuid)}/activity`, {
      query: params,
    });
  },
};
