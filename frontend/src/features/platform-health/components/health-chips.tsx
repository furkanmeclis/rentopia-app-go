"use client";

import { StatusChip } from "@/components/common/status-chip";
import type {
  HealthSeverity,
  HealthStatus,
} from "@/features/platform-health/types";
import { useLocale } from "@/providers/locale-provider";

const STATUS_TONE = {
  healthy: "success",
  degraded: "warning",
  failing: "danger",
  unknown: "default",
} as const;

const STATUS_LABEL_KEYS: Record<HealthStatus, string> = {
  healthy: "platform_health.status.healthy",
  degraded: "platform_health.status.degraded",
  failing: "platform_health.status.failing",
  unknown: "platform_health.status.unknown",
};

const SEVERITY_TONE = {
  critical: "danger",
  warning: "warning",
  info: "default",
} as const;

export const SEVERITY_LABEL_KEYS: Record<HealthSeverity, string> = {
  critical: "platform_health.severity.critical",
  warning: "platform_health.severity.warning",
  info: "platform_health.severity.info",
};

export const HEALTH_SEVERITIES: HealthSeverity[] = [
  "critical",
  "warning",
  "info",
];

export function HealthStatusChip({ status }: { status: HealthStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONE[status]}
    />
  );
}

export function SeverityChip({ severity }: { severity: HealthSeverity }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(SEVERITY_LABEL_KEYS[severity])}
      tone={SEVERITY_TONE[severity]}
    />
  );
}
