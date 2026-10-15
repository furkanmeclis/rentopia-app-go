"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { BellOff, BellRing } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  CONSENT_CHANNELS,
  CONSENT_CHANNEL_LABEL_KEYS,
  CONSENT_SOURCES,
  CONSENT_SOURCE_LABEL_KEYS,
} from "@/features/customers/constants";
import {
  useCustomerConsents,
  useRecordCustomerConsent,
} from "@/features/customers/hooks/use-customers";
import type {
  ConsentChannel,
  ConsentSource,
  CustomerConsent,
} from "@/features/customers/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-consent-form";

type ConsentRequest = { channel: ConsentChannel; action: "grant" | "withdraw" };

type Values = {
  source: ConsentSource;
  text_version: string;
  date: string;
  note: string;
};

function ConsentDialog({
  customerUuid,
  request,
  onClose,
}: {
  customerUuid: string;
  request: ConsentRequest | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const record = useRecordCustomerConsent(customerUuid);
  const granting = request?.action === "grant";
  const schema = useMemo(
    () =>
      z
        .object({
          source: z.enum(["counter", "web", "phone", "import", "other"]),
          text_version: z.string().max(64),
          date: z.string(),
          note: z.string().max(2000),
        })
        .superRefine((v, ctx) => {
          if (granting && !v.text_version.trim()) {
            ctx.addIssue({
              code: "custom",
              path: ["text_version"],
              message: t("customers.validation.required"),
            });
          }
        }),
    [granting, t],
  );

  return (
    <AppDialog
      open={Boolean(request)}
      onOpenChange={(open) => {
        if (!open && !record.isPending) onClose();
      }}
      title={
        request
          ? t(
              granting
                ? "customers.consents.grant_title"
                : "customers.consents.withdraw_title",
              { channel: t(CONSENT_CHANNEL_LABEL_KEYS[request.channel]) },
            )
          : ""
      }
      description={t("customers.consents.dialog_description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            variant={granting ? "default" : "destructive"}
            disabled={record.isPending}
          >
            {granting
              ? t("customers.consents.grant")
              : t("customers.consents.withdraw")}
          </Button>
        </>
      }
    >
      {request ? (
        <AppForm<Values>
          key={`${request.channel}-${request.action}`}
          id={FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{
            source: "counter",
            text_version: "",
            date: "",
            note: "",
          }}
          onSubmit={async (values) => {
            await record.mutateAsync({
              channel: request.channel,
              action: request.action,
              source: values.source,
              text_version: values.text_version.trim(),
              note: values.note.trim(),
              // A back-dated record keeps noon of that day (Türkiye).
              at: values.date ? `${values.date}T12:00:00+03:00` : null,
            });
            onClose();
          }}
        >
          <AppSelect
            name="source"
            label={t("customers.consents.source")}
            options={CONSENT_SOURCES.map((value) => ({
              value,
              label: t(CONSENT_SOURCE_LABEL_KEYS[value]),
            }))}
          />
          {granting ? (
            <AppInput
              name="text_version"
              label={t("customers.consents.text_version")}
              description={t("customers.consents.text_version_hint")}
              placeholder="ETK-2026-01"
            />
          ) : null}
          <AppDatePicker
            name="date"
            label={t("customers.consents.date")}
            description={t("customers.consents.date_hint")}
          />
          <AppTextarea name="note" label={t("customers.fields.note")} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * "İzinler" tab: current marketing consent per channel (SMS, e-posta,
 * arama, WhatsApp) with grant / withdraw actions and the full history
 * (DataTable). Consent is a separate record with source and text version.
 */
export function CustomerConsentsTab({
  customerUuid,
  readOnly,
}: {
  customerUuid: string;
  readOnly: boolean;
}) {
  const { t, locale } = useLocale();
  const query = useCustomerConsents(customerUuid);
  const [request, setRequest] = useState<ConsentRequest | null>(null);
  const write = permissions.tenant.customers.write;

  const actorName = (actor: CustomerConsent["recorded_by"]) =>
    actor ? `${actor.name} ${actor.surname}`.trim() : "—";

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerConsent>({
          id: "channel",
          accessorFn: (row) => row.channel,
          labelKey: "customers.consents.channel",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CONSENT_CHANNELS.map((value) => ({
            value,
            labelKey: CONSENT_CHANNEL_LABEL_KEYS[value],
            label: t(CONSENT_CHANNEL_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(CONSENT_CHANNEL_LABEL_KEYS[row.original.channel])}
            </span>
          ),
        }),
        createColumn<CustomerConsent>({
          id: "active",
          accessorFn: (row) => String(row.active),
          labelKey: "customers.consents.state",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("customers.consents.active")
                  : t("customers.consents.withdrawn")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<CustomerConsent>({
          accessorKey: "granted_at",
          labelKey: "customers.consents.granted_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.granted_at, undefined, locale),
        }),
        createColumn<CustomerConsent>({
          id: "withdrawn_at",
          accessorFn: (row) => row.withdrawn_at ?? "",
          labelKey: "customers.consents.withdrawn_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.withdrawn_at
              ? datetime(row.original.withdrawn_at, undefined, locale)
              : "—",
        }),
        createColumn<CustomerConsent>({
          id: "source",
          accessorFn: (row) => row.source,
          labelKey: "customers.consents.source",
          enableSorting: true,
          cell: ({ row }) => t(CONSENT_SOURCE_LABEL_KEYS[row.original.source]),
        }),
        createColumn<CustomerConsent>({
          accessorKey: "text_version",
          labelKey: "customers.consents.text_version",
          enableSorting: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.text_version || "—"}</code>
          ),
        }),
        createColumn<CustomerConsent>({
          id: "recorded_by",
          accessorFn: (row) => actorName(row.recorded_by),
          labelKey: "customers.consents.recorded_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => actorName(row.original.recorded_by),
        }),
        createColumn<CustomerConsent>({
          accessorKey: "note",
          labelKey: "customers.fields.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
      ] as ColumnDef<CustomerConsent, unknown>[],
    [locale, t],
  );

  const current = query.data?.current ?? [];
  const items = query.data?.items ?? [];

  return (
    <div className="space-y-6">
      <EntitySectionCard title={t("customers.consents.current_title")}>
        <p className="text-muted-foreground mb-4 text-sm">
          {t("customers.consents.help")}
        </p>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {current.map((state) => (
            <div
              key={state.channel}
              className="flex flex-col gap-3 rounded-lg border p-4"
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">
                  {t(CONSENT_CHANNEL_LABEL_KEYS[state.channel])}
                </span>
                <StatusChip
                  label={
                    state.granted
                      ? t("customers.consents.active")
                      : t("customers.consents.none")
                  }
                  tone={state.granted ? "success" : "default"}
                />
              </div>
              <p className="text-muted-foreground text-xs">
                {state.granted && state.since
                  ? t("customers.consents.since", {
                      date: datetime(state.since, undefined, locale),
                      version: state.text_version,
                    })
                  : t("customers.consents.no_consent")}
              </p>
              {readOnly ? null : (
                <PermissionGuard permission={write}>
                  <Button
                    type="button"
                    size="sm"
                    variant={state.granted ? "outline" : "default"}
                    onClick={() =>
                      setRequest({
                        channel: state.channel,
                        action: state.granted ? "withdraw" : "grant",
                      })
                    }
                  >
                    {state.granted ? (
                      <BellOff className="size-4" />
                    ) : (
                      <BellRing className="size-4" />
                    )}
                    {state.granted
                      ? t("customers.consents.withdraw")
                      : t("customers.consents.grant")}
                  </Button>
                </PermissionGuard>
              )}
            </div>
          ))}
        </div>
      </EntitySectionCard>

      <EntitySectionCard
        title={t("customers.consents.history_title")}
        badge={query.data ? items.length : undefined}
      >
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(row) => row.uuid}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          emptyTitle={t("customers.consents.empty_title")}
          emptyDescription={t("customers.consents.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-customer-consents-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>

      <ConsentDialog
        customerUuid={customerUuid}
        request={request}
        onClose={() => setRequest(null)}
      />
    </div>
  );
}
