"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Copy, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useFieldArray, useFormContext } from "react-hook-form";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { AppCheckbox, AppForm, AppInput } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { INSPECTION_TEMPLATES_RESOURCE } from "@/features/inspections/constants";
import {
  useInspectionTemplates,
  useSaveTemplate,
} from "@/features/inspections/hooks/use-inspections";
import type {
  InspectionTemplate,
  InspectionTemplateRequest,
} from "@/features/inspections/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "inspection-template-form";
const KEY_RE = /^[a-z][a-z0-9_]{0,31}$/;

function RowsEditor({
  name,
  withPrice,
}: {
  name: "photo_slots" | "checklist";
  withPrice: boolean;
}) {
  const { t } = useLocale();
  const { control } = useFormContext<InspectionTemplateRequest>();
  const { fields, append, remove } = useFieldArray({ control, name });
  return (
    <div className="grid gap-3">
      {fields.map((field, index) => (
        <div
          key={field.id}
          className="grid gap-2 rounded-md border p-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
        >
          <AppInput
            name={`${name}.${index}.label`}
            label={t("inspections.templates.label")}
          />
          <AppInput
            name={`${name}.${index}.key`}
            label={t("inspections.templates.key")}
            autoCapitalize="none"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t("inspections.templates.remove_row")}
            onClick={() => remove(index)}
          >
            <Trash2 className="size-4" />
          </Button>
          {withPrice ? (
            <AppInput
              name={`${name}.${index}.price`}
              inputMode="decimal"
              label={t("inspections.templates.price")}
              description={t("inspections.templates.price_hint")}
            />
          ) : null}
          <AppCheckbox
            name={`${name}.${index}.required`}
            label={t("inspections.templates.required")}
          />
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="justify-self-start"
        onClick={() =>
          append(
            (withPrice
              ? { key: "", label: "", required: true, price: "" }
              : { key: "", label: "", required: true }) as never,
          )
        }
      >
        <Plus className="size-4" />
        {t(
          withPrice
            ? "inspections.templates.add_item"
            : "inspections.templates.add_slot",
        )}
      </Button>
    </div>
  );
}

/**
 * "Teslim / iade formları" settings: template versions (DataTable); saving
 * creates the next active version, forms keep the version they used.
 */
export function InspectionTemplatesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const listState = useServerListState({
    initialSort: "-version",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, INSPECTION_TEMPLATES_RESOURCE),
  });
  const query = useInspectionTemplates(listState.params);
  const save = useSaveTemplate();
  const [editing, setEditing] = useState<InspectionTemplate | null>(null);
  const canSave = can(permissions.tenant.rentals.close);
  const total = query.data?.total ?? 0;

  const schema = useMemo(() => {
    const key = z
      .string()
      .trim()
      .regex(KEY_RE, t("inspections.templates.key_invalid"));
    const label = z
      .string()
      .trim()
      .min(1, t("inspections.templates.label_required"))
      .max(80);
    return z.object({
      name: z.string().trim().min(2).max(160),
      photo_slots: z
        .array(z.object({ key, label, required: z.boolean() }))
        .min(1, t("inspections.templates.slots_required")),
      checklist: z.array(
        z.object({
          key,
          label,
          required: z.boolean(),
          price: z
            .string()
            .trim()
            .regex(/^(\d+([.,]\d{1,2})?)?$/, t("rentals.validation.amount")),
        }),
      ),
    });
  }, [t]);

  const columns = useMemo(
    () =>
      [
        createColumn<InspectionTemplate>({
          accessorKey: "version",
          labelKey: "inspections.templates.columns.version",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              v{row.original.version} · {row.original.name}
            </span>
          ),
        }),
        createColumn<InspectionTemplate>({
          accessorKey: "status",
          labelKey: "inspections.templates.columns.status",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.status === "active"
                  ? "inspections.templates.active"
                  : "inspections.templates.archived",
              )}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<InspectionTemplate>({
          id: "slots",
          accessorFn: (row) => row.photo_slots.length,
          labelKey: "inspections.templates.columns.slots",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.photo_slots.map((s) => s.label).join(", "),
        }),
        createColumn<InspectionTemplate>({
          id: "checklist",
          accessorFn: (row) => row.checklist.length,
          labelKey: "inspections.templates.columns.checklist",
          enableSorting: false,
          cell: ({ row }) => row.original.checklist.length,
        }),
        createColumn<InspectionTemplate>({
          accessorKey: "inspections",
          labelKey: "inspections.templates.columns.inspections",
          enableSorting: false,
          cell: ({ row }) => row.original.inspections,
        }),
        createColumn<InspectionTemplate>({
          accessorKey: "created_at",
          labelKey: "inspections.templates.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<InspectionTemplate>({
          id: "actions",
          labelKey: "inspections.templates.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            canSave ? (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("inspections.templates.new_version"),
                    icon: Copy,
                    onSelect: () => setEditing(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<InspectionTemplate, unknown>[],
    [canSave, locale, t],
  );

  const active = query.data?.items.find((i) => i.status === "active");
  return (
    <EntityPage
      title={t("inspections.templates.title")}
      description={t("inspections.templates.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("inspections.templates.title") },
      ]}
      actions={
        canSave && active ? (
          <Button type="button" onClick={() => setEditing(active)}>
            <Plus className="size-4" />
            {t("inspections.templates.new_version")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("inspections.templates.empty")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-inspection-templates-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <AppDialog
        open={Boolean(editing)}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setEditing(null);
        }}
        title={t("inspections.templates.new_version")}
        description={t("inspections.templates.new_version_hint")}
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        footer={
          <>
            <Button variant="outline" onClick={() => setEditing(null)}>
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={save.isPending}>
              {t("inspections.templates.save")}
            </Button>
          </>
        }
      >
        {editing ? (
          <AppForm<InspectionTemplateRequest>
            id={FORM_ID}
            className="grid gap-4"
            schema={schema as never}
            defaultValues={{
              name: editing.name,
              photo_slots: editing.photo_slots,
              checklist: editing.checklist,
            }}
            onSubmit={async (values) => {
              await save.mutateAsync({
                ...values,
                checklist: values.checklist.map((c) => ({
                  ...c,
                  price: c.price.replace(",", "."),
                })),
              });
              setEditing(null);
            }}
          >
            <AppInput name="name" label={t("inspections.templates.name")} />
            <Separator />
            <p className="text-sm font-medium">
              {t("inspections.templates.slots")}
            </p>
            <RowsEditor name="photo_slots" withPrice={false} />
            <Separator />
            <p className="text-sm font-medium">
              {t("inspections.templates.checklist")}
            </p>
            <RowsEditor name="checklist" withPrice />
          </AppForm>
        ) : null}
      </AppDialog>
    </EntityPage>
  );
}
