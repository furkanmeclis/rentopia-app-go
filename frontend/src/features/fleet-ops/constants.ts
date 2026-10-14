import type {
  BlockKind,
  BlockState,
  TransferStatus,
} from "@/features/fleet-ops/types";

/** Engine / live resource slugs. */
export const TRANSFERS_RESOURCE = "tenant.transfers" as const;
export const ONE_WAY_RULES_RESOURCE = "tenant.one_way_rules" as const;
export const VEHICLE_BLOCKS_RESOURCE = "tenant.vehicle_blocks" as const;

export const BLOCK_KINDS: BlockKind[] = [
  "maintenance",
  "damage",
  "cleaning",
  "out_of_service",
  "other",
];

export const BLOCK_STATES: BlockState[] = ["scheduled", "active", "ended"];

/** Kinds whose block can also set the vehicle status ("other" cannot). */
export const STATUS_SYNC_KINDS: BlockKind[] = [
  "maintenance",
  "damage",
  "cleaning",
  "out_of_service",
];

export const TRANSFER_STATUSES: TransferStatus[] = [
  "planned",
  "in_transit",
  "completed",
  "cancelled",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const BLOCK_KIND_LABEL_KEYS: Record<BlockKind, string> = {
  maintenance: "vehicle_blocks.kind.maintenance",
  damage: "vehicle_blocks.kind.damage",
  cleaning: "vehicle_blocks.kind.cleaning",
  out_of_service: "vehicle_blocks.kind.out_of_service",
  other: "vehicle_blocks.kind.other",
};

export const BLOCK_STATE_LABEL_KEYS: Record<BlockState, string> = {
  scheduled: "vehicle_blocks.state.scheduled",
  active: "vehicle_blocks.state.active",
  ended: "vehicle_blocks.state.ended",
};

export const BLOCK_STATE_TONES: Record<
  BlockState,
  "default" | "success" | "warning" | "danger"
> = {
  scheduled: "warning",
  active: "danger",
  ended: "default",
};

export const TRANSFER_STATUS_LABEL_KEYS: Record<TransferStatus, string> = {
  planned: "transfers.status.planned",
  in_transit: "transfers.status.in_transit",
  completed: "transfers.status.completed",
  cancelled: "transfers.status.cancelled",
};

export const TRANSFER_STATUS_TONES: Record<
  TransferStatus,
  "default" | "success" | "warning" | "danger"
> = {
  planned: "warning",
  in_transit: "default",
  completed: "success",
  cancelled: "danger",
};
