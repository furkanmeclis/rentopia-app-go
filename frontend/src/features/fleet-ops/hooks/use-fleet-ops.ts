"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { fleetOpsService } from "@/features/fleet-ops/services/fleet-ops.service";
import type {
  ListBlocksParams,
  ListOneWayRulesParams,
  ListTransfersParams,
  OneWayRuleCreateRequest,
  OneWayRulePatchRequest,
  TransferAction,
  TransferActionRequest,
  TransferCreateRequest,
  TransferPatchRequest,
  VehicleBlockPatchRequest,
  VehicleBlockReleaseRequest,
  VehicleBlockRequest,
  VehicleBlockResult,
} from "@/features/fleet-ops/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehicleKeys } from "@/features/vehicles/hooks/use-vehicles";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const fleetOpsKeys = {
  all: (slug: string) => ["tenant", slug, "fleet-ops"] as const,
  blocks: (slug: string, vehicleUuid: string) =>
    [...fleetOpsKeys.all(slug), "blocks", vehicleUuid] as const,
  blockList: (
    slug: string,
    vehicleUuid: string,
    params: Record<string, unknown>,
  ) => [...fleetOpsKeys.blocks(slug, vehicleUuid), params] as const,
  transfers: (slug: string) =>
    [...fleetOpsKeys.all(slug), "transfers"] as const,
  transferList: (slug: string, params: Record<string, unknown>) =>
    [...fleetOpsKeys.transfers(slug), "list", params] as const,
  transfersMeta: (slug: string) =>
    [...fleetOpsKeys.transfers(slug), "meta"] as const,
  transferFacets: (slug: string) =>
    [...fleetOpsKeys.transfers(slug), "facets"] as const,
  assignees: (slug: string, q: string) =>
    [...fleetOpsKeys.transfers(slug), "assignees", q] as const,
  rules: (slug: string) => [...fleetOpsKeys.all(slug), "rules"] as const,
  ruleList: (slug: string, params: Record<string, unknown>) =>
    [...fleetOpsKeys.rules(slug), "list", params] as const,
  rulesMeta: (slug: string) => [...fleetOpsKeys.rules(slug), "meta"] as const,
  ruleFacets: (slug: string) =>
    [...fleetOpsKeys.rules(slug), "facets"] as const,
};

/**
 * Blocks and transfers change the vehicle (status, current branch): refresh
 * the fleet-ops cache and the vehicles cache.
 */
function useInvalidateFleetOps() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: fleetOpsKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: vehicleKeys.all(slug) }),
    ]);
}

/** Warns about the open transfers (later: reservations) a block overlaps. */
function useAffectedToast() {
  const { t } = useLocale();
  return (result: VehicleBlockResult) => {
    if (result.affected.length === 0) return;
    appToast.warning(
      t("vehicle_blocks.toast.affected", {
        count: result.affected.length,
        items: result.affected.map((item) => item.label).join(", "),
      }),
    );
  };
}

// ------------------------------------------------------------------ blocks

export function useVehicleBlocks(
  vehicleUuid: string,
  params: ListBlocksParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.blockList(slug, vehicleUuid, params),
    queryFn: () => fleetOpsService.blocks(slug, vehicleUuid, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCreateBlock(vehicleUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  const affected = useAffectedToast();
  return useMutation({
    mutationFn: (body: VehicleBlockRequest) =>
      fleetOpsService.createBlock(slug, vehicleUuid, body),
    onSuccess: (result) => {
      appToast.success(
        result.status_changed
          ? t("vehicle_blocks.toast.created_with_status")
          : t("vehicle_blocks.toast.created"),
      );
      affected(result);
      void invalidate();
    },
  });
}

export function useUpdateBlock(vehicleUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  const affected = useAffectedToast();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: VehicleBlockPatchRequest;
    }) => fleetOpsService.updateBlock(slug, vehicleUuid, uuid, body),
    onSuccess: (result) => {
      appToast.success(t("vehicle_blocks.toast.updated"));
      affected(result);
      void invalidate();
    },
  });
}

export function useReleaseBlock(vehicleUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: VehicleBlockReleaseRequest;
    }) => fleetOpsService.releaseBlock(slug, vehicleUuid, uuid, body),
    onSuccess: () => {
      appToast.success(t("vehicle_blocks.toast.released"));
      void invalidate();
    },
  });
}

export function useDeleteBlock(vehicleUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  return useMutation({
    mutationFn: (uuid: string) =>
      fleetOpsService.removeBlock(slug, vehicleUuid, uuid),
    onSuccess: () => {
      appToast.success(t("vehicle_blocks.toast.deleted"));
      void invalidate();
    },
  });
}

// --------------------------------------------------------------- transfers

export function useTransfersList(params: ListTransfersParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.transferList(slug, params),
    queryFn: () => fleetOpsService.transfers(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useTransfersMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.transfersMeta(slug),
    queryFn: () => fleetOpsService.transfersMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useTransferFacets(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.transferFacets(slug),
    queryFn: () => fleetOpsService.transferFacets(slug),
    staleTime: 60_000,
    enabled,
  });
}

export function useTransferAssignees(q: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.assignees(slug, q),
    queryFn: () => fleetOpsService.assignees(slug, q),
    staleTime: 60_000,
    enabled,
  });
}

export function useCreateTransfer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  return useMutation({
    mutationFn: (body: TransferCreateRequest) =>
      fleetOpsService.createTransfer(slug, body),
    onSuccess: () => {
      appToast.success(t("transfers.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateTransfer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: TransferPatchRequest;
    }) => fleetOpsService.updateTransfer(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("transfers.toast.updated"));
      void invalidate();
    },
  });
}

const ACTION_TOAST_KEYS: Record<TransferAction, string> = {
  start: "transfers.toast.started",
  complete: "transfers.toast.completed",
  cancel: "transfers.toast.cancelled",
};

export function useTransferAction() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleetOps();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
      body,
    }: {
      uuid: string;
      action: TransferAction;
      body: TransferActionRequest;
    }) => fleetOpsService.transferAction(slug, uuid, action, body),
    onSuccess: (_, { action }) => {
      appToast.success(t(ACTION_TOAST_KEYS[action]));
      void invalidate();
    },
  });
}

// ----------------------------------------------------------- one-way rules

export function useOneWayRulesList(params: ListOneWayRulesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.ruleList(slug, params),
    queryFn: () => fleetOpsService.rules(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useOneWayRulesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.rulesMeta(slug),
    queryFn: () => fleetOpsService.rulesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useOneWayRuleFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetOpsKeys.ruleFacets(slug),
    queryFn: () => fleetOpsService.ruleFacets(slug),
    staleTime: 60_000,
  });
}

function useInvalidateRules() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: fleetOpsKeys.rules(slug) });
}

export function useCreateOneWayRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRules();
  return useMutation({
    mutationFn: (body: OneWayRuleCreateRequest) =>
      fleetOpsService.createRule(slug, body),
    onSuccess: () => {
      appToast.success(t("one_way_rules.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateOneWayRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRules();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: OneWayRulePatchRequest;
    }) => fleetOpsService.updateRule(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("one_way_rules.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteOneWayRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRules();
  return useMutation({
    mutationFn: (uuid: string) => fleetOpsService.removeRule(slug, uuid),
    onSuccess: () => {
      appToast.success(t("one_way_rules.toast.deleted"));
      void invalidate();
    },
  });
}
