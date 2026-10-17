import type {
  ChannelAccount,
  ChannelMapping,
  ChannelMappingFacets,
  ChannelMappingInput,
  ChannelMappingPage,
  ChannelMessageDetail,
  ChannelMessageFacets,
  ChannelMessagePage,
  ChannelReconciliation,
  ChannelSettingsInput,
  ChannelSimulateInput,
  ChannelSyncRun,
  ChannelSyncRunPage,
  ListChannelMappingsParams,
  ListChannelMessagesParams,
  ListChannelSyncRunsParams,
} from "@/features/channels/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const ACCOUNTS = "/v1/tenant/channel-accounts";
const MAPPINGS = "/v1/tenant/channel-mappings";
const MESSAGES = "/v1/tenant/channel-messages";
const RUNS = "/v1/tenant/channel-sync-runs";

/**
 * Channel manager (`tenantRequest` only). Sync actions whose answers the
 * screens render themselves are `silent`.
 */
export const channelsService = {
  accounts(slug: string) {
    return tenantRequest<ChannelAccount[]>(slug, "GET", ACCOUNTS);
  },
  updateAccount(slug: string, uuid: string, body: ChannelSettingsInput) {
    return tenantRequest<ChannelAccount>(slug, "PATCH", `${ACCOUNTS}/${uuid}`, {
      body,
    });
  },
  push(slug: string, uuid: string) {
    return tenantRequest<ChannelSyncRun>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/push`,
      { silent: true },
    );
  },
  pull(slug: string, uuid: string) {
    return tenantRequest<ChannelSyncRun>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/pull`,
      { silent: true },
    );
  },
  simulate(slug: string, uuid: string, body: ChannelSimulateInput) {
    return tenantRequest<ChannelSyncRun>(
      slug,
      "POST",
      `${ACCOUNTS}/${uuid}/simulate`,
      { body, silent: true },
    );
  },
  reconciliation(slug: string, uuid: string, from: string, to: string) {
    return tenantRequest<ChannelReconciliation>(
      slug,
      "GET",
      `${ACCOUNTS}/${uuid}/reconciliation`,
      { query: { from, to } },
    );
  },
  mappings(slug: string, params: ListChannelMappingsParams) {
    return tenantRequest<ChannelMappingPage>(slug, "GET", MAPPINGS, {
      query: params,
    });
  },
  mappingFacets(slug: string) {
    return tenantRequest<ChannelMappingFacets>(
      slug,
      "GET",
      `${MAPPINGS}/facets`,
    );
  },
  createMapping(slug: string, body: ChannelMappingInput) {
    return tenantRequest<ChannelMapping>(slug, "POST", MAPPINGS, { body });
  },
  updateMapping(slug: string, uuid: string, body: ChannelMappingInput) {
    return tenantRequest<ChannelMapping>(slug, "PATCH", `${MAPPINGS}/${uuid}`, {
      body,
    });
  },
  deleteMapping(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${MAPPINGS}/${uuid}`,
    );
  },
  messages(slug: string, params: ListChannelMessagesParams) {
    return tenantRequest<ChannelMessagePage>(slug, "GET", MESSAGES, {
      query: params,
    });
  },
  messageFacets(slug: string) {
    return tenantRequest<ChannelMessageFacets>(
      slug,
      "GET",
      `${MESSAGES}/facets`,
    );
  },
  message(slug: string, uuid: string) {
    return tenantRequest<ChannelMessageDetail>(
      slug,
      "GET",
      `${MESSAGES}/${uuid}`,
    );
  },
  retryMessage(slug: string, uuid: string) {
    return tenantRequest<ChannelMessageDetail>(
      slug,
      "POST",
      `${MESSAGES}/${uuid}/retry`,
      { silent: true },
    );
  },
  ignoreMessage(slug: string, uuid: string, note: string) {
    return tenantRequest<ChannelMessageDetail>(
      slug,
      "POST",
      `${MESSAGES}/${uuid}/ignore`,
      { body: { note }, silent: true },
    );
  },
  runs(slug: string, params: ListChannelSyncRunsParams) {
    return tenantRequest<ChannelSyncRunPage>(slug, "GET", RUNS, {
      query: params,
    });
  },
};
