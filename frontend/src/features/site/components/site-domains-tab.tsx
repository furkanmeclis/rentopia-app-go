"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Copy, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useSiteDomainActions,
  useSiteDomains,
} from "@/features/site/hooks/use-site";
import type { SiteDomain } from "@/features/site/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const MAX_DOMAINS = 3;

const statusTone = {
  pending: "warning",
  verified: "success",
  failed: "danger",
  disabled: "default",
} as const;

const tlsTone = {
  pending: "default",
  provisioning: "warning",
  active: "success",
  failed: "danger",
  manual: "warning",
} as const;

function copy(value: string, done: string) {
  void navigator.clipboard?.writeText(value).then(() => appToast.success(done));
}

/** The DNS records the customer creates at their DNS provider. */
function DnsRecords({ domain }: { domain: SiteDomain }) {
  const { t } = useLocale();
  return (
    <div className="space-y-2">
      <p className="text-muted-foreground text-xs">
        {t("site.domains.records_hint")}
      </p>
      <div className="divide-y rounded-md border text-sm">
        {domain.dns_records.map((rec) => (
          <div
            key={`${rec.type}-${rec.name}`}
            className="grid gap-1 p-3 sm:grid-cols-[4rem_1fr_1fr_auto] sm:items-center"
          >
            <span className="font-medium">{rec.type}</span>
            <code className="text-xs break-all">{rec.name}</code>
            <code className="text-xs break-all">{rec.value}</code>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("site.domains.copy_value")}
              onClick={() => copy(rec.value, t("site.domains.toast.copied"))}
            >
              <Copy className="size-4" />
            </Button>
          </div>
        ))}
      </div>
      {domain.tls_instructions ? (
        <p className="text-muted-foreground text-xs">
          {domain.tls_instructions}
        </p>
      ) : null}
    </div>
  );
}

/**
 * Alan adları (TEC-731): the organization's own domains for its branded
 * site. A domain resolves only after DNS verification (TXT + target);
 * HTTPS is provisioned by the platform.
 */
export function SiteDomainsTab({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const domains = useSiteDomains();
  const { add, verify, remove } = useSiteDomainActions();
  const [adding, setAdding] = useState(false);
  const [host, setHost] = useState("");
  const [addError, setAddError] = useState<string | null>(null);
  const [selected, setSelected] = useState<SiteDomain | null>(null);
  const [removing, setRemoving] = useState<SiteDomain | null>(null);
  const rows = useMemo(() => domains.data ?? [], [domains.data]);
  const atLimit = rows.length >= MAX_DOMAINS;

  const submit = () => {
    setAddError(null);
    add.mutate(host.trim(), {
      onSuccess: (domain) => {
        setAdding(false);
        setHost("");
        setSelected(domain);
      },
      onError: (error) => {
        const code = isApiError(error) ? error.code : "";
        setAddError(
          t(
            code === "DOMAIN_LIMIT" || code === "DOMAIN_TAKEN"
              ? `site.domains.errors.${code}`
              : "site.domains.errors.invalid",
          ),
        );
      },
    });
  };

  const columns = useMemo(
    () =>
      [
        createColumn<SiteDomain>({
          accessorKey: "host",
          labelKey: "site.domains.columns.host",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.host}</span>
              {row.original.last_error ? (
                <span className="text-destructive text-xs">
                  {row.original.last_error}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<SiteDomain>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "site.domains.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.domains.status.${row.original.status}`)}
              tone={statusTone[row.original.status]}
            />
          ),
        }),
        createColumn<SiteDomain>({
          id: "tls",
          accessorFn: (row) => row.tls_status,
          labelKey: "site.domains.columns.tls",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.domains.tls.${row.original.tls_status}`)}
              tone={tlsTone[row.original.tls_status]}
            />
          ),
        }),
        createColumn<SiteDomain>({
          id: "actions",
          accessorFn: () => "",
          labelKey: "site.domains.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "records",
                  label: t("site.domains.show_records"),
                  icon: Copy,
                  onSelect: () => setSelected(row.original),
                },
                {
                  id: "verify",
                  label: t("site.domains.verify"),
                  icon: RefreshCw,
                  disabled: !canManage || verify.isPending,
                  onSelect: () => verify.mutate(row.original.uuid),
                },
                {
                  id: "remove",
                  label: t("site.domains.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  disabled: !canManage,
                  onSelect: () => setRemoving(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<SiteDomain, unknown>[],
    [canManage, t, verify],
  );

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("site.domains.description")}
      </p>
      <EntitySectionCard
        title={t("site.domains.title")}
        badge={`${rows.length}/${MAX_DOMAINS}`}
        actions={
          canManage ? (
            <Button
              type="button"
              size="sm"
              disabled={atLimit}
              onClick={() => {
                setAddError(null);
                setAdding(true);
              }}
            >
              <Plus className="size-4" />
              {t("site.domains.add")}
            </Button>
          ) : null
        }
      >
        <EntityTable
          isLoading={domains.isLoading}
          isError={domains.isError}
          errorDescription={t("site.domains.load_failed")}
          onRetry={() => void domains.refetch()}
          manual={{ filtering: false, sorting: false, pagination: false }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void domains.refetch()}
              refreshDisabled={domains.isFetching}
            />
          }
          columns={columns}
          data={rows}
          getRowId={(row) => row.uuid}
          emptyTitle={t("site.domains.empty")}
          features={{
            persistKey: "tenant-site-domains-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>

      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("site.domains.add_title")}</DialogTitle>
            <DialogDescription>{t("site.domains.add_hint")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="site-domain-host">
              {t("site.domains.columns.host")}
            </Label>
            <Input
              id="site-domain-host"
              value={host}
              placeholder="rezervasyon.firmaniz.com"
              autoComplete="off"
              aria-invalid={addError ? true : undefined}
              aria-describedby={addError ? "site-domain-host-error" : undefined}
              onChange={(e) => setHost(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && host.trim()) submit();
              }}
            />
            {addError ? (
              <p
                id="site-domain-host-error"
                className="text-destructive text-sm"
              >
                {addError}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setAdding(false)}
            >
              {t("site.domains.cancel")}
            </Button>
            <Button
              type="button"
              disabled={!host.trim() || add.isPending}
              onClick={submit}
            >
              {t("site.domains.add")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => !open && setSelected(null)}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t("site.domains.records_title", { host: selected?.host ?? "" })}
            </DialogTitle>
          </DialogHeader>
          {selected ? <DnsRecords domain={selected} /> : null}
        </DialogContent>
      </Dialog>

      <DeleteDialog
        open={removing !== null}
        title={t("site.domains.remove_title", { host: removing?.host ?? "" })}
        description={t("site.domains.remove_description")}
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() =>
          removing &&
          remove.mutate(removing.uuid, { onSettled: () => setRemoving(null) })
        }
      />
    </div>
  );
}
