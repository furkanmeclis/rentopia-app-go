"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { AppInput, AppSwitch } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  useCategories,
  useSaveCategory,
} from "@/features/finance/hooks/use-finance";
import { errorText } from "@/features/finance/lib/format";
import type { ExpenseCategory } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const CODE_RE = /^[a-z0-9][a-z0-9_]{1,31}$/;

type CategoryValues = {
  code: string;
  name: string;
  sort_order: string;
  is_active: boolean;
};

function CategoryDialog({
  category,
  open,
  onOpenChange,
}: {
  category: ExpenseCategory | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const save = useSaveCategory();
  const editing = Boolean(category);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<CategoryValues>
      key={category?.uuid ?? "new"}
      open={open}
      onOpenChange={close}
      formId="finance-category-form"
      title={
        editing
          ? t("finance.categories.edit_title")
          : t("finance.categories.create_title")
      }
      description={t("finance.categories.form_description")}
      submitLabel={t("form.save")}
      pending={save.isPending}
      problem={problem}
      schema={z.object({
        code: editing
          ? z.string()
          : z
              .string()
              .trim()
              .regex(CODE_RE, t("finance.validation.category_code")),
        name: z
          .string()
          .trim()
          .min(1, t("finance.validation.required"))
          .max(128),
        sort_order: z
          .string()
          .regex(/^\d{0,4}$/, t("finance.validation.sort_order")),
        is_active: z.boolean(),
      })}
      defaultValues={{
        code: category?.code ?? "",
        name: category?.name ?? "",
        sort_order: String(category?.sort_order ?? 100),
        is_active: category?.is_active ?? true,
      }}
      onSubmit={async (v) => {
        try {
          await save.mutateAsync({
            uuid: category?.uuid,
            body: {
              code: editing ? undefined : v.code.trim(),
              name: v.name.trim(),
              sort_order: v.sort_order ? Number(v.sort_order) : undefined,
              is_active: editing ? v.is_active : undefined,
            },
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="code"
          label={t("finance.fields.code")}
          description={
            editing
              ? t("finance.categories.code_fixed")
              : t("finance.categories.code_hint")
          }
          disabled={editing}
        />
        <AppInput
          name="sort_order"
          label={t("finance.fields.sort_order")}
          inputMode="numeric"
        />
      </div>
      <AppInput name="name" label={t("finance.fields.name")} />
      {editing ? (
        <AppSwitch name="is_active" label={t("finance.fields.is_active")} />
      ) : null}
    </FinanceFormDialog>
  );
}

/** "Gider kategorileri": the organization's expense categories. */
export function CategoriesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const query = useCategories(false);
  const [editing, setEditing] = useState<ExpenseCategory | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const columns = useMemo(
    () =>
      [
        createColumn<ExpenseCategory>({
          accessorKey: "code",
          labelKey: "finance.columns.code",
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
              {row.original.code}
            </code>
          ),
        }),
        createColumn<ExpenseCategory>({
          accessorKey: "name",
          labelKey: "finance.columns.name",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{row.original.name}</span>
              {row.original.is_system ? (
                <Badge variant="secondary">
                  {t("finance.categories.system")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<ExpenseCategory>({
          accessorKey: "expense_count",
          labelKey: "finance.columns.expense_count",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.expense_count}</span>
          ),
        }),
        createColumn<ExpenseCategory>({
          accessorKey: "sort_order",
          labelKey: "finance.columns.sort_order",
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.sort_order}</span>
          ),
        }),
        createColumn<ExpenseCategory>({
          id: "status",
          accessorFn: (row) => (row.is_active ? "active" : "archived"),
          labelKey: "finance.columns.status",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.is_active
                  ? t("finance.account_status.active")
                  : t("finance.categories.inactive")
              }
              tone={row.original.is_active ? "success" : "default"}
            />
          ),
        }),
        createColumn<ExpenseCategory>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("finance.actions.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.finance.expenses.write,
                  onSelect: () => setEditing(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<ExpenseCategory, unknown>[],
    [t],
  );

  return (
    <EntityPage
      title={t("finance.categories.title")}
      description={t("finance.categories.description")}
      permission={permissions.tenant.finance.expenses.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.categories.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("finance.categories.create")}
          permission={permissions.tenant.finance.expenses.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("finance.categories.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-finance-categories-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <CategoryDialog
        category={editing}
        open={createOpen || Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) {
            setCreateOpen(false);
            setEditing(null);
          }
        }}
      />
    </EntityPage>
  );
}
