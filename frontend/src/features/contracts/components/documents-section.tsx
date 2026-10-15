"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  Eye,
  FilePlus2,
  RefreshCw,
  Send,
} from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { permissions } from "@/config/permissions";
import { HtmlPreview } from "@/features/contracts/components/html-preview";
import {
  CONTRACT_KINDS,
  DOCUMENT_STATUS_LABEL_KEYS,
  DOCUMENT_STATUS_TONES,
  KIND_LABEL_KEYS,
  PDF_STATUS_LABEL_KEYS,
  VERSION_STATUS_LABEL_KEYS,
  VERSION_STATUS_TONES,
  placeholderLabel,
} from "@/features/contracts/constants";
import {
  useApproveVersion,
  useDocument,
  useDocumentVersion,
  useDocuments,
  useDownloadPdf,
  useIssueVersion,
  usePrepareDocument,
  useRefreshDocument,
} from "@/features/contracts/hooks/use-contracts";
import type {
  ContractDocument,
  DocumentApproval,
  DocumentListItem,
  DocumentVersionInfo,
  VersionStatus,
} from "@/features/contracts/types";
import { actorName } from "@/features/reservations/components/reservation-cells";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const APPROVE_FORM_ID = "document-approve-form";

function versionStatusText(t: (key: string) => string, status: string) {
  const key = VERSION_STATUS_LABEL_KEYS[status as VersionStatus];
  return key ? t(key) : status;
}

function pdfText(t: (key: string) => string, status: string) {
  const key = PDF_STATUS_LABEL_KEYS[status];
  return key ? t(key) : status;
}

type ApproveValues = {
  approver_name: string;
  party: "customer" | "staff";
  note: string;
};

/**
 * Records an in-person approval of one version. The content hash of the
 * version on screen goes with it: a newer version answers
 * CONTENT_CHANGED and nothing is recorded.
 */
function ApproveDialog({
  document,
  version,
  open,
  onOpenChange,
}: {
  document: ContractDocument;
  version: DocumentVersionInfo;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const approve = useApproveVersion();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        approver_name: z
          .string()
          .trim()
          .min(2, t("contracts.validation.approver"))
          .max(160, t("contracts.validation.approver")),
        party: z.enum(["customer", "staff"]),
        note: z.string().max(500),
      }),
    [t],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !approve.isPending) close();
      }}
      title={t("contracts.approve.title")}
      description={t("contracts.approve.description", {
        version: version.version,
        hash: version.content_hash.slice(0, 12),
      })}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={APPROVE_FORM_ID}
            disabled={approve.isPending}
          >
            {t("contracts.actions.record_approval")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {problem ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertTitle>{t("contracts.problems.title")}</AlertTitle>
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        {open ? (
          <AppForm<ApproveValues>
            id={APPROVE_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{ approver_name: "", party: "customer", note: "" }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await approve.mutateAsync({
                  doc: document.uuid,
                  ver: version.uuid,
                  body: {
                    content_hash: version.content_hash,
                    party: values.party,
                    method: "in_person",
                    approver_name: values.approver_name.trim(),
                    note: values.note.trim() || undefined,
                  },
                });
                close();
              } catch (error) {
                setProblem(
                  error instanceof ApiError && error.code === "CONTENT_CHANGED"
                    ? t("contracts.problems.content_changed")
                    : error instanceof Error
                      ? error.message
                      : String(error),
                );
              }
            }}
          >
            <AppInput
              name="approver_name"
              label={t("contracts.fields.approver_name")}
            />
            <AppSelect
              name="party"
              label={t("contracts.fields.party")}
              options={[
                { value: "customer", label: t("contracts.parties.customer") },
                { value: "staff", label: t("contracts.parties.staff") },
              ]}
            />
            <AppInput name="note" label={t("contracts.fields.note")} />
            <p className="text-muted-foreground text-xs">
              {t("contracts.approve.method_hint")}
            </p>
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

/** Document dialog: versions, the selected version's HTML, approvals. */
function DocumentDialog({
  documentUuid,
  onClose,
  canWrite,
}: {
  documentUuid: string | null;
  onClose: () => void;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const docQuery = useDocument(documentUuid);
  const doc = docQuery.data;
  const [selection, setSelection] = useState<RowSelectionState>({});
  const selected =
    Object.keys(selection).find((id) => selection[id]) ??
    doc?.current_version?.uuid ??
    null;
  const versionQuery = useDocumentVersion(documentUuid, selected);
  const version = versionQuery.data;
  const issue = useIssueVersion();
  const download = useDownloadPdf();
  const [approving, setApproving] = useState(false);
  const isCurrent = Boolean(
    version && doc?.current_version?.uuid === version.uuid,
  );
  const canApprove =
    canWrite &&
    isCurrent &&
    doc?.status !== "void" &&
    (version?.status === "draft" || version?.status === "issued");

  const versionColumns = useMemo(
    () =>
      [
        createSelectColumnDef<DocumentVersionInfo>(),
        createColumn<DocumentVersionInfo>({
          accessorKey: "version",
          labelKey: "contracts.versions.columns.version",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => `v${row.original.version}`,
        }),
        createColumn<DocumentVersionInfo>({
          accessorKey: "status",
          labelKey: "contracts.versions.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(VERSION_STATUS_LABEL_KEYS[row.original.status])}
              tone={VERSION_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<DocumentVersionInfo>({
          id: "changed",
          accessorFn: (row) => row.changed_keys.join(","),
          labelKey: "contracts.versions.columns.changed",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.changed_keys.length
              ? row.original.changed_keys
                  .map((k) => placeholderLabel(t, k))
                  .join(", ")
              : "—",
        }),
        createColumn<DocumentVersionInfo>({
          accessorKey: "created_at",
          labelKey: "contracts.versions.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM HH:mm", locale),
        }),
      ] as ColumnDef<DocumentVersionInfo, unknown>[],
    [locale, t],
  );

  const approvalColumns = useMemo(
    () =>
      [
        createColumn<DocumentApproval>({
          accessorKey: "approver_name",
          labelKey: "contracts.approvals.columns.approver",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.approver_name}</span>
          ),
        }),
        createColumn<DocumentApproval>({
          accessorKey: "party",
          labelKey: "contracts.approvals.columns.party",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.party === "staff"
              ? t("contracts.parties.staff")
              : t("contracts.parties.customer"),
        }),
        createColumn<DocumentApproval>({
          accessorKey: "version_number",
          labelKey: "contracts.approvals.columns.version",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span>v{row.original.version_number}</span>
              <span className="text-muted-foreground font-mono text-[10px]">
                {row.original.content_hash.slice(0, 12)}
              </span>
            </span>
          ),
        }),
        createColumn<DocumentApproval>({
          accessorKey: "valid",
          labelKey: "contracts.approvals.columns.valid",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.valid ? (
              <StatusChip
                label={t("contracts.approvals.valid")}
                tone="success"
              />
            ) : (
              <span className="flex flex-col">
                <StatusChip label={t("contracts.approvals.invalidated")} />
                <span className="text-muted-foreground text-xs">
                  {row.original.invalidated_reason}
                </span>
              </span>
            ),
        }),
        createColumn<DocumentApproval>({
          accessorKey: "approved_at",
          labelKey: "contracts.approvals.columns.approved_at",
          enableSorting: false,
          cell: ({ row }) =>
            `${datetime(row.original.approved_at, "d MMM yyyy HH:mm", locale)} · ${actorName(row.original.recorded_by)}`,
        }),
      ] as ColumnDef<DocumentApproval, unknown>[],
    [locale, t],
  );

  const close = () => {
    setSelection({});
    onClose();
  };

  return (
    <AppDialog
      open={Boolean(documentUuid)}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={
        doc
          ? `${t(KIND_LABEL_KEYS[doc.kind])} · ${doc.reservation.reference}`
          : t("contracts.documents.title")
      }
      description={t("contracts.documents.dialog_description")}
      className="sm:max-w-5xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("contracts.actions.close")}
          </Button>
          {version &&
          version.pdf_status !== "pending" &&
          can(permissions.tenant.document.download) ? (
            <Button
              type="button"
              variant="outline"
              disabled={download.isPending}
              onClick={() =>
                download.mutate({
                  doc: documentUuid ?? "",
                  ver: version.uuid,
                  fallbackName: `${doc?.reservation.reference ?? "document"}-v${version.version}.pdf`,
                })
              }
            >
              <Download className="size-4" />
              {t("contracts.actions.download_pdf")}
            </Button>
          ) : null}
          {canWrite && isCurrent && version?.status === "draft" ? (
            <Button
              type="button"
              variant="outline"
              disabled={issue.isPending}
              onClick={() =>
                issue.mutate({ doc: documentUuid ?? "", ver: version.uuid })
              }
            >
              <Send className="size-4" />
              {t("contracts.actions.issue")}
            </Button>
          ) : null}
          {canApprove ? (
            <Button type="button" onClick={() => setApproving(true)}>
              <CheckCircle2 className="size-4" />
              {t("contracts.actions.record_approval")}
            </Button>
          ) : null}
        </>
      }
    >
      <div className="max-h-[75vh] space-y-4 overflow-y-auto">
        {doc ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <StatusChip
              label={t(DOCUMENT_STATUS_LABEL_KEYS[doc.status])}
              tone={DOCUMENT_STATUS_TONES[doc.status]}
            />
            {version ? (
              <>
                <Badge variant="outline">
                  {version.template_name} · v{version.template_version}
                </Badge>
                <Badge variant="outline" className="font-mono">
                  SHA-256 {version.content_hash.slice(0, 16)}…
                </Badge>
                <Badge variant="outline">
                  PDF: {pdfText(t, version.pdf_status)}
                </Badge>
              </>
            ) : null}
          </div>
        ) : null}
        {version && !isCurrent ? (
          <Alert>
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {t("contracts.documents.old_version")}
            </AlertDescription>
          </Alert>
        ) : null}
        {version?.pdf_status === "failed" && version.pdf_error ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertDescription>{version.pdf_error}</AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-4 lg:grid-cols-[2fr_3fr]">
          <div className="space-y-4">
            <EntityTable
              columns={versionColumns}
              data={doc?.versions ?? []}
              getRowId={(row) => row.uuid}
              isLoading={docQuery.isLoading}
              emptyTitle={t("contracts.versions.empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              state={{
                rowSelection: selected ? { [selected]: true } : {},
                onRowSelectionChange: (updater) => {
                  const current = selected ? { [selected]: true } : {};
                  const next =
                    typeof updater === "function" ? updater(current) : updater;
                  const ids = Object.keys(next).filter(
                    (id) => next[id] && id !== selected,
                  );
                  if (ids.length) setSelection({ [ids[0]]: true });
                },
              }}
              features={{
                persistKey: "tenant-document-versions-v1",
                rowSelection: true,
                globalFilter: false,
                sorting: false,
              }}
            />
            <EntityTable
              columns={approvalColumns}
              data={doc?.approvals ?? []}
              getRowId={(row) => row.uuid}
              emptyTitle={t("contracts.approvals.empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-document-approvals-v1",
                rowSelection: false,
                globalFilter: false,
                sorting: false,
              }}
            />
          </div>
          <div>
            {version ? (
              <HtmlPreview
                html={version.html}
                title={t("contracts.documents.preview")}
              />
            ) : null}
          </div>
        </div>
      </div>
      {doc && version ? (
        <ApproveDialog
          document={doc}
          version={version}
          open={approving}
          onOpenChange={setApproving}
        />
      ) : null}
    </AppDialog>
  );
}

/**
 * Documents of a reservation (and its rental): prepare a document from
 * the active template of a kind, see its immutable versions, record the
 * in-person approval bound to the content hash, download the PDF through
 * the API.
 */
export function DocumentsSection({
  reservationUuid,
}: {
  reservationUuid: string;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite =
    can(permissions.tenant.contracts.write) ||
    can(permissions.tenant.rentals.write);
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 });
  const query = useDocuments({
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
    reservation: reservationUuid,
  });
  const prepare = usePrepareDocument();
  const refresh = useRefreshDocument();
  const download = useDownloadPdf();
  const [openDoc, setOpenDoc] = useState<string | null>(null);
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const existing = new Set(
    items.filter((d) => d.status !== "void").map((d) => d.kind),
  );

  const columns = useMemo(
    () =>
      [
        createColumn<DocumentListItem>({
          accessorKey: "kind",
          labelKey: "contracts.documents.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {t(KIND_LABEL_KEYS[row.original.kind])}
            </span>
          ),
        }),
        createColumn<DocumentListItem>({
          accessorKey: "status",
          labelKey: "contracts.documents.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(DOCUMENT_STATUS_LABEL_KEYS[row.original.status])}
              tone={DOCUMENT_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<DocumentListItem>({
          id: "version",
          accessorFn: (row) => row.version_number,
          labelKey: "contracts.documents.columns.version",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.version_uuid
              ? `v${row.original.version_number} · ${versionStatusText(t, row.original.version_status)}`
              : "—",
        }),
        createColumn<DocumentListItem>({
          id: "template",
          accessorFn: (row) => row.template_name,
          labelKey: "contracts.documents.columns.template",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.template_name
              ? `${row.original.template_name} · v${row.original.template_version}`
              : "—",
        }),
        createColumn<DocumentListItem>({
          accessorKey: "pdf_status",
          labelKey: "contracts.documents.columns.pdf",
          enableSorting: false,
          cell: ({ row }) => pdfText(t, row.original.pdf_status),
        }),
        createColumn<DocumentListItem>({
          id: "rental",
          accessorFn: (row) => row.rental?.reference ?? "",
          labelKey: "contracts.documents.columns.rental",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.rental?.reference ?? "—",
        }),
        createColumn<DocumentListItem>({
          accessorKey: "updated_at",
          labelKey: "contracts.documents.columns.updated_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.updated_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<DocumentListItem>({
          id: "actions",
          labelKey: "contracts.documents.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const d = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("contracts.actions.open"),
                    icon: Eye,
                    onSelect: () => setOpenDoc(d.uuid),
                  },
                  ...(canWrite && d.status !== "void"
                    ? [
                        {
                          id: "refresh",
                          label: t("contracts.actions.refresh"),
                          icon: RefreshCw,
                          onSelect: () => refresh.mutate(d.uuid),
                        },
                      ]
                    : []),
                  ...(d.version_uuid && d.pdf_status !== "pending"
                    ? [
                        {
                          id: "pdf",
                          label: t("contracts.actions.download_pdf"),
                          icon: Download,
                          permission: permissions.tenant.document.download,
                          onSelect: () =>
                            download.mutate({
                              doc: d.uuid,
                              ver: d.version_uuid ?? "",
                              fallbackName: `${d.reservation.reference}-v${d.version_number}.pdf`,
                            }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<DocumentListItem, unknown>[],
    [canWrite, download, locale, refresh, t],
  );

  return (
    <EntitySectionCard
      title={t("contracts.documents.title")}
      badge={query.data ? total : undefined}
      actions={
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={prepare.isPending}
              >
                <FilePlus2 className="size-4" />
                {t("contracts.actions.prepare")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {CONTRACT_KINDS.map((kind) => (
                <DropdownMenuItem
                  key={kind}
                  onSelect={() =>
                    prepare.mutate(
                      { kind, reservation_uuid: reservationUuid },
                      {
                        onSuccess: (out) => setOpenDoc(out.document.uuid),
                      },
                    )
                  }
                >
                  {t(KIND_LABEL_KEYS[kind])}
                  {existing.has(kind) ? (
                    <Badge variant="outline" className="ml-auto text-[10px]">
                      {t("contracts.documents.exists")}
                    </Badge>
                  ) : null}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setOpenDoc(row.uuid)}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("contracts.documents.empty")}
        emptyDescription={t("contracts.documents.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          persistKey: "tenant-reservation-documents-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
      <DocumentDialog
        documentUuid={openDoc}
        canWrite={canWrite}
        onClose={() => setOpenDoc(null)}
      />
    </EntitySectionCard>
  );
}
