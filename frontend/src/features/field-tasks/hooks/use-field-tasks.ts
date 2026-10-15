"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  fieldTasksService,
  type FieldTaskStep,
} from "@/features/field-tasks/services/field-tasks.service";
import type {
  FieldTaskCreateRequest,
  FieldTaskPatchRequest,
  ListFieldTasksParams,
} from "@/features/field-tasks/types";
import { rentalKeys } from "@/features/rentals/hooks/use-rentals";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const fieldTaskKeys = {
  all: (slug: string) => ["tenant", slug, "field-tasks"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...fieldTaskKeys.all(slug), "list", params] as const,
  mine: (slug: string, params: Record<string, unknown>) =>
    [...fieldTaskKeys.all(slug), "mine", params] as const,
  meta: (slug: string) => [...fieldTaskKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...fieldTaskKeys.all(slug), "facets"] as const,
  assignees: (slug: string) =>
    [...fieldTaskKeys.all(slug), "assignees"] as const,
  detail: (slug: string, uuid: string) =>
    [...fieldTaskKeys.all(slug), "detail", uuid] as const,
  events: (slug: string, uuid: string) =>
    [...fieldTaskKeys.all(slug), "events", uuid] as const,
  deliveryPoints: (slug: string, branch?: string, product?: string) =>
    [...fieldTaskKeys.all(slug), "delivery-points", branch, product] as const,
};

/** Task writes touch rentals' task lists too. */
export function useInvalidateFieldTasks() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: fieldTaskKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: rentalKeys.all(slug) }),
    ]);
}

export function useFieldTasks(params: ListFieldTasksParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.list(slug, params),
    queryFn: () => fieldTasksService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useMyFieldTasks(params: ListFieldTasksParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.mine(slug, params),
    queryFn: () => fieldTasksService.mine(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useFieldTasksMeta(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.meta(slug),
    queryFn: () => fieldTasksService.meta(slug),
    staleTime: 5 * 60_000,
    enabled,
  });
}

export function useFieldTaskFacets(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.facets(slug),
    queryFn: () => fieldTasksService.facets(slug),
    staleTime: 60_000,
    enabled,
  });
}

export function useFieldTaskAssignees(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.assignees(slug),
    queryFn: () => fieldTasksService.assignees(slug),
    staleTime: 60_000,
    enabled,
  });
}

export function useFieldTask(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.detail(slug, uuid),
    queryFn: () => fieldTasksService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useFieldTaskEvents(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.events(slug, uuid),
    queryFn: () => fieldTasksService.events(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useDeliveryPoints(branch?: string, product?: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fieldTaskKeys.deliveryPoints(slug, branch, product),
    queryFn: () => fieldTasksService.deliveryPoints(slug, branch, product),
    staleTime: 60_000,
  });
}

const STEP_TOAST_KEYS: Record<FieldTaskStep, string> = {
  start: "field_tasks.toast.started",
  "report-issue": "field_tasks.toast.issue_reported",
  complete: "field_tasks.toast.completed",
  cancel: "field_tasks.toast.cancelled",
};

export function useFieldTaskStep() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFieldTasks();
  return useMutation({
    mutationFn: ({
      uuid,
      step,
      note,
    }: {
      uuid: string;
      step: FieldTaskStep;
      note?: string;
    }) => fieldTasksService.step(slug, uuid, step, note),
    onSuccess: (_, v) => {
      appToast.success(t(STEP_TOAST_KEYS[v.step]));
      void invalidate();
    },
  });
}

export function useAssignFieldTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFieldTasks();
  return useMutation({
    mutationFn: ({
      uuid,
      userUuid,
    }: {
      uuid: string;
      userUuid: string | null;
    }) => fieldTasksService.assign(slug, uuid, userUuid),
    onSuccess: (_, v) => {
      appToast.success(
        t(
          v.userUuid
            ? "field_tasks.toast.assigned"
            : "field_tasks.toast.unassigned",
        ),
      );
      void invalidate();
    },
  });
}

export function useCreateFieldTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFieldTasks();
  return useMutation({
    mutationFn: (body: FieldTaskCreateRequest) =>
      fieldTasksService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("field_tasks.toast.created"));
      void invalidate();
    },
  });
}

export function usePatchFieldTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFieldTasks();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: FieldTaskPatchRequest;
    }) => fieldTasksService.patch(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("field_tasks.toast.updated"));
      void invalidate();
    },
  });
}
