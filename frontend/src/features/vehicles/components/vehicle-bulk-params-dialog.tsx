"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import type { BulkActionDef } from "@/features/bulk-engine";
import { useGroupsList } from "@/features/fleet/hooks/use-fleet";
import {
  STATUS_LABEL_KEYS,
  VEHICLE_STATUSES,
} from "@/features/vehicles/constants";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-bulk-params";

/** Bulk actions that need an argument, and the param they collect. */
const PARAM_ACTIONS: Record<
  string,
  "status" | "branch_uuid" | "rental_group_uuid"
> = {
  set_status: "status",
  move_branch: "branch_uuid",
  add_rental_group: "rental_group_uuid",
  replace_rental_group: "rental_group_uuid",
};

const HINT_KEYS: Record<string, string> = {
  set_status: "vehicles.bulk_params.set_status_hint",
  move_branch: "vehicles.bulk_params.move_branch_hint",
  add_rental_group: "vehicles.bulk_params.add_rental_group_hint",
  replace_rental_group: "vehicles.bulk_params.replace_rental_group_hint",
};

type Request = {
  action: BulkActionDef;
  resolve: (params: Record<string, string> | null) => void;
};

type Values = { value: string; note: string };

function ParamsForm({
  request,
  onDone,
}: {
  request: Request;
  onDone: (params: Record<string, string> | null) => void;
}) {
  const { t } = useLocale();
  const param = PARAM_ACTIONS[request.action.id]!;
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const groups = useGroupsList(
    { limit: 100, offset: 0, sort: "code", is_active: "true" },
    param === "rental_group_uuid",
  );
  const schema = useMemo(
    () =>
      z.object({
        value: z.string().min(1, t("vehicles.bulk_params.required")),
        note: z.string().max(2000),
      }),
    [t],
  );

  let options: { value: string; label: string }[] = [];
  let label = "";
  if (param === "status") {
    label = t("vehicles.fields.status");
    options = VEHICLE_STATUSES.map((value) => ({
      value,
      label: t(STATUS_LABEL_KEYS[value]),
    }));
  } else if (param === "branch_uuid") {
    label = t("vehicles.fields.current_branch");
    options = (branches.data?.items ?? []).map((b) => ({
      value: b.uuid,
      label: `${b.code} · ${b.name}`,
    }));
  } else {
    label = t("vehicles.fields.rental_group");
    options = (groups.data?.items ?? []).map((g) => ({
      value: g.uuid,
      label: `${g.code} · ${g.name}`,
    }));
  }

  return (
    <AppForm<Values>
      id={FORM_ID}
      className="space-y-4"
      schema={schema}
      defaultValues={{ value: "", note: "" }}
      onSubmit={async (values) => {
        const params: Record<string, string> = { [param]: values.value };
        if (param === "status" && values.note.trim()) {
          params.note = values.note.trim();
        }
        onDone(params);
      }}
    >
      <p className="text-muted-foreground text-sm">
        {t(HINT_KEYS[request.action.id] ?? "vehicles.bulk_params.required")}
      </p>
      <AppSelect
        name="value"
        label={label}
        placeholder={t("vehicles.bulk_params.placeholder")}
        options={options}
      />
      {param === "status" ? (
        <AppTextarea name="note" label={t("vehicles.fields.note")} />
      ) : null}
    </AppForm>
  );
}

/**
 * Collects the params of the vehicles bulk actions (status, branch, rental
 * group). `resolveParams` plugs into `BulkActionMenu`.
 */
export function useVehicleBulkParams(): {
  resolveParams: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
  dialog: ReactNode;
} {
  const { t } = useLocale();
  const [request, setRequest] = useState<Request | null>(null);

  const resolveParams = useCallback((action: BulkActionDef) => {
    if (!PARAM_ACTIONS[action.id]) return Promise.resolve(undefined);
    return new Promise<Record<string, string> | null>((resolve) =>
      setRequest({ action, resolve }),
    );
  }, []);

  const finish = (params: Record<string, string> | null) => {
    request?.resolve(params);
    setRequest(null);
  };

  const dialog = (
    <AppDialog
      open={Boolean(request)}
      onOpenChange={(open) => {
        if (!open) finish(null);
      }}
      title={request ? t(request.action.label_key) : ""}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID}>
            {t("vehicles.bulk_params.continue")}
          </Button>
        </>
      }
    >
      {request ? (
        <ParamsForm key={request.action.id} request={request} onDone={finish} />
      ) : null}
    </AppDialog>
  );

  return { resolveParams, dialog };
}
