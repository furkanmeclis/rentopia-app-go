"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, Circle } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import { AppForm, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RentalStatusChip } from "@/features/rentals/components/rental-cells";
import { rentalsService } from "@/features/rentals/services/rentals.service";
import type { RentalStatus } from "@/features/rentals/types";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { stationText, tollMoney } from "@/features/tolls/components/toll-cells";
import {
  useTollCandidates,
  useTollDecision,
} from "@/features/tolls/hooks/use-tolls";
import type { TollCandidate, TollPass } from "@/features/tolls/types";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const ASSIGN_FORM = "toll-assign-form";

/** Text of a failed decision: validation details or the API message. */
export function tollProblemText(
  t: (key: string) => string,
  error: unknown,
): string {
  if (!(error instanceof ApiError)) return String(error);
  const code = error.details[0]?.code;
  if (code === "not_found" && error.details[0]?.field === "rental_uuid") {
    return t("tolls.problems.rental_not_found");
  }
  if (code === "invalid_state" && error.details[0]?.field === "rental_uuid") {
    return t("tolls.problems.rental_not_started");
  }
  if (error.status === 409 && code === "already_ignored") {
    return t("tolls.problems.already_ignored");
  }
  if (error.details.length > 0) {
    return error.details
      .map((d) => (d.field ? `${d.field}: ${d.message}` : d.message))
      .join(" · ");
  }
  return error.message;
}

function PassSummary({ pass }: { pass: TollPass }) {
  const { locale } = useLocale();
  return (
    <div className="bg-muted/40 grid gap-1 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium">{stationText(pass)}</span>
        <span className="font-semibold tabular-nums">
          {tollMoney(pass.amount, pass.currency, locale)}
        </span>
      </div>
      <div className="text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs">
        <span className="tabular-nums">
          {datetime(pass.passed_at, "d MMM yyyy HH:mm", locale)}
        </span>
        <span className="font-mono">{pass.plate || pass.tag_number}</span>
        <span className="font-mono">{pass.external_id}</span>
      </div>
    </div>
  );
}

/**
 * Assign a pass to a rental: the rentals of the vehicle around the pass
 * time (a DataTable; "contains the pass" marks the actual interval), or a
 * search over all rentals; a mandatory reason. The old charge is reversed
 * and the new one created by the server.
 */
export function AssignTollDialog({
  pass,
  onClose,
}: {
  pass: TollPass | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const candidates = useTollCandidates(pass?.uuid ?? null);
  const decision = useTollDecision();
  const [selected, setSelected] = useState("");
  const [searched, setSearched] = useState<ComboboxOption[]>([]);
  const [problem, setProblem] = useState<string | null>(null);

  const close = () => {
    setSelected("");
    setProblem(null);
    onClose();
  };

  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("tolls.validation.reason"))
          .max(500, t("tolls.validation.reason")),
      }),
    [t],
  );

  const loadOptions = useCallback(
    async (q: string) => {
      const page = await rentalsService.list(slug, {
        limit: 10,
        offset: 0,
        q: q || undefined,
        status: "active,return_check,returned",
        sort: "-created_at",
      });
      const opts = page.items.map((r) => ({
        value: r.uuid,
        label: `${r.reference} · ${r.vehicle.plate}`,
        description: r.customer.display_name,
      }));
      setSearched(opts);
      return opts;
    },
    [slug],
  );

  const items = candidates.data?.items ?? [];
  const columns = useMemo(
    () =>
      [
        createColumn<TollCandidate>({
          id: "pick",
          labelKey: "tolls.assign.columns.pick",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            row.original.uuid === selected ? (
              <CheckCircle2 className="text-primary size-4" aria-hidden />
            ) : (
              <Circle className="text-muted-foreground size-4" aria-hidden />
            ),
        }),
        createColumn<TollCandidate>({
          accessorKey: "reference",
          labelKey: "tolls.assign.columns.reference",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-mono text-xs font-medium">
                {row.original.reference}
              </span>
              {row.original.contains_pass ? (
                <Badge variant="success" className="px-1.5 py-0 text-[10px]">
                  {t("tolls.assign.contains_pass")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<TollCandidate>({
          accessorKey: "customer",
          labelKey: "tolls.assign.columns.customer",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<TollCandidate>({
          id: "window",
          labelKey: "tolls.assign.columns.window",
          enableSorting: false,
          cell: ({ row }) => {
            const c = row.original;
            const from = c.actual_pickup_at ?? c.planned_pickup_at;
            const to = c.actual_return_at ?? c.planned_return_at;
            return (
              <span className="text-xs tabular-nums">
                {datetime(from, "d MMM HH:mm", locale)} →{" "}
                {datetime(to, "d MMM HH:mm", locale)}
                {!c.actual_pickup_at ? (
                  <span className="text-muted-foreground">
                    {" "}
                    ({t("tolls.assign.planned")})
                  </span>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<TollCandidate>({
          accessorKey: "status",
          labelKey: "tolls.assign.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <RentalStatusChip status={row.original.status as RentalStatus} />
          ),
        }),
      ] as ColumnDef<TollCandidate, unknown>[],
    [locale, selected, t],
  );

  return (
    <AppDialog
      open={Boolean(pass)}
      onOpenChange={(next) => {
        if (!next && !decision.isPending) close();
      }}
      title={t("tolls.assign.title")}
      description={t("tolls.assign.description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={decision.isPending}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={ASSIGN_FORM}
            disabled={decision.isPending || !selected}
          >
            {t("tolls.actions.assign")}
          </Button>
        </>
      }
    >
      {pass ? (
        <div className="space-y-4">
          <PassSummary pass={pass} />
          <div className="space-y-2">
            <Label>{t("tolls.assign.candidates")}</Label>
            <EntityTable
              columns={columns}
              data={items}
              getRowId={(row) => row.uuid}
              onRowClick={(row) => setSelected(row.uuid)}
              isLoading={candidates.isLoading}
              isError={candidates.isError}
              onRetry={() => void candidates.refetch()}
              emptyTitle={t("tolls.assign.no_candidates")}
              emptyDescription={t("tolls.assign.no_candidates_hint")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-toll-candidates-v1",
                rowSelection: false,
                globalFilter: false,
                sorting: false,
                columnVisibility: false,
                density: false,
              }}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="toll-assign-search">
              {t("tolls.assign.search_label")}
            </Label>
            <AsyncCombobox
              id="toll-assign-search"
              value={searched.some((o) => o.value === selected) ? selected : ""}
              onValueChange={(value) => setSelected(value)}
              loadOptions={loadOptions}
              placeholder={t("tolls.assign.search_placeholder")}
              searchPlaceholder={t("tolls.assign.search_hint")}
              emptyText={t("tolls.assign.search_empty")}
              clearable
            />
          </div>
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<{ reason: string }>
            id={ASSIGN_FORM}
            schema={schema}
            defaultValues={{ reason: "" }}
            onSubmit={async (values) => {
              if (!selected) return;
              setProblem(null);
              try {
                await decision.mutateAsync({
                  action: "assign",
                  uuid: pass.uuid,
                  rental_uuid: selected,
                  reason: values.reason.trim(),
                });
                close();
              } catch (error) {
                setProblem(tollProblemText(t, error));
              }
            }}
          >
            <AppTextarea
              name="reason"
              rows={2}
              label={t("tolls.fields.reason")}
              description={t("tolls.fields.reason_hint")}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

export type ReasonAction = "unassign" | "ignore" | "rematch";

const REASON_TITLES: Record<ReasonAction, string> = {
  unassign: "tolls.decisions.unassign_title",
  ignore: "tolls.decisions.ignore_title",
  rematch: "tolls.decisions.rematch_title",
};

const REASON_DESCRIPTIONS: Record<ReasonAction, string> = {
  unassign: "tolls.decisions.unassign_description",
  ignore: "tolls.decisions.ignore_description",
  rematch: "tolls.decisions.rematch_description",
};

const REASON_CONFIRMS: Record<ReasonAction, string> = {
  unassign: "tolls.actions.unassign",
  ignore: "tolls.actions.ignore",
  rematch: "tolls.actions.rematch",
};

/** Unassign / ignore / rematch with the mandatory reason. */
export function TollReasonDialog({
  target,
  onClose,
}: {
  target: { pass: TollPass; action: ReasonAction } | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const decision = useTollDecision();
  return (
    <ReasonDialog
      open={Boolean(target)}
      title={target ? t(REASON_TITLES[target.action]) : ""}
      description={target ? t(REASON_DESCRIPTIONS[target.action]) : undefined}
      label={t("tolls.fields.reason")}
      confirmLabel={target ? t(REASON_CONFIRMS[target.action]) : ""}
      destructive={target?.action !== "rematch"}
      pending={decision.isPending}
      onCancel={onClose}
      onConfirm={(reason) => {
        if (!target) return;
        decision.mutate(
          { action: target.action, uuid: target.pass.uuid, reason },
          {
            onSuccess: onClose,
            onError: (error) => appToast.error(tollProblemText(t, error)),
          },
        );
      }}
    />
  );
}
