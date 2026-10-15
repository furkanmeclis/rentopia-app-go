"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  Ban,
  Check,
  Copy,
  ExternalLink,
  Link2,
  MessageSquare,
} from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityRowActions, EntityTable } from "@/components/entity";
import { AppForm, AppSelect, AppSwitch } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  LINK_EXPIRY_OPTIONS,
  LINK_STATUS_LABEL_KEYS,
  LINK_STATUS_TONES,
  SIGNER_ROLE_LABEL_KEYS,
} from "@/features/contracts/constants";
import {
  useApprovalLinks,
  useCreateApprovalLink,
  useDocumentSigners,
  useRevokeApprovalLink,
  useSendApprovalLink,
} from "@/features/contracts/hooks/use-remote-approval";
import type {
  ApprovalLink,
  ContractDocument,
  DocumentVersionInfo,
} from "@/features/contracts/types";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const CREATE_FORM_ID = "document-remote-link-form";

function copyText(text: string, done: () => void) {
  void navigator.clipboard.writeText(text).then(done);
}

/** A link URL with copy / open. */
export function ApprovalLinkBox({
  url,
  expiresAt,
  phone,
}: {
  url: string;
  expiresAt: string;
  phone: string;
}) {
  const { t, locale } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-2 rounded-md border p-3">
      <p className="text-sm font-medium">
        {t("contracts.remote.created_title")}
      </p>
      <div className="flex gap-2">
        <Input readOnly value={url} aria-label={t("contracts.remote.url")} />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("contracts.remote.copy")}
          onClick={() =>
            copyText(url, () => {
              setCopied(true);
              window.setTimeout(() => setCopied(false), 2000);
            })
          }
        >
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("contracts.remote.open_link")}
          onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
        >
          <ExternalLink className="size-4" />
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("contracts.remote.summary", {
          phone,
          expires: datetime(expiresAt, "d MMM yyyy HH:mm", locale),
        })}
      </p>
    </div>
  );
}

type CreateValues = {
  signer: string;
  expires: string;
  send_sms: boolean;
};

/**
 * Creates the remote approval link of the version on screen: the content
 * hash goes with it (a newer version answers CONTENT_CHANGED). The person
 * receives the SMS code on the number shown (masked).
 */
export function CreateLinkDialog({
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
  const signersQuery = useDocumentSigners(document.uuid, open);
  const create = useCreateApprovalLink();
  const [problem, setProblem] = useState<string | null>(null);
  const [created, setCreated] = useState<ApprovalLink | null>(null);
  const signers = useMemo(
    () => signersQuery.data?.signers ?? [],
    [signersQuery.data],
  );
  const smsAvailable = signersQuery.data?.sms_available ?? false;
  const options = signers
    .filter((s) => s.has_phone)
    .map((s) => ({
      value:
        s.role === "driver" && s.driver_uuid
          ? `driver:${s.driver_uuid}`
          : "renter",
      label: `${s.name} · ${t(SIGNER_ROLE_LABEL_KEYS[s.role] ?? s.role)} · ${s.phone_masked}`,
    }));
  const withoutPhone = signers.filter((s) => !s.has_phone);
  const schema = useMemo(
    () =>
      z.object({
        signer: z.string().min(1, t("contracts.remote.validation.signer")),
        expires: z.string().min(1),
        send_sms: z.boolean(),
      }),
    [t],
  );
  const close = () => {
    setProblem(null);
    setCreated(null);
    onOpenChange(false);
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !create.isPending) close();
      }}
      title={t("contracts.remote.create_title")}
      description={t("contracts.remote.create_description", {
        version: version.version,
        hash: version.content_hash.slice(0, 12),
      })}
      className="sm:max-w-lg"
      footer={
        created ? (
          <Button type="button" onClick={close}>
            {t("contracts.actions.close")}
          </Button>
        ) : (
          <>
            <Button type="button" variant="outline" onClick={close}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={CREATE_FORM_ID}
              disabled={create.isPending || options.length === 0}
            >
              <Link2 className="size-4" />
              {t("contracts.remote.create")}
            </Button>
          </>
        )
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
        {created?.url ? (
          <ApprovalLinkBox
            url={created.url}
            expiresAt={created.expires_at}
            phone={created.phone_masked}
          />
        ) : null}
        {!created && signersQuery.isSuccess && options.length === 0 ? (
          <Alert>
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {t("contracts.remote.no_phone")}
            </AlertDescription>
          </Alert>
        ) : null}
        {open && !created && signersQuery.isSuccess ? (
          <AppForm<CreateValues>
            id={CREATE_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              signer: options[0]?.value ?? "",
              expires: "1440",
              send_sms: smsAvailable,
            }}
            onSubmit={async (values) => {
              setProblem(null);
              const [role, driver] = values.signer.split(":");
              try {
                const out = await create.mutateAsync({
                  doc: document.uuid,
                  ver: version.uuid,
                  body: {
                    content_hash: version.content_hash,
                    signer_role: role === "driver" ? "driver" : "renter",
                    driver_uuid: driver || undefined,
                    expires_in_minutes: Number(values.expires),
                    send_sms: values.send_sms,
                  },
                });
                setCreated(out.link);
                if (out.sms_error) {
                  setProblem(t("contracts.remote.sms_failed"));
                }
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
            <AppSelect
              name="signer"
              label={t("contracts.remote.fields.signer")}
              options={options}
            />
            <AppSelect
              name="expires"
              label={t("contracts.remote.fields.expires")}
              options={LINK_EXPIRY_OPTIONS.map((o) => ({
                value: String(o.minutes),
                label: t(o.labelKey),
              }))}
            />
            <AppSwitch
              name="send_sms"
              label={t("contracts.remote.fields.send_sms")}
              description={
                smsAvailable
                  ? t("contracts.remote.fields.send_sms_hint")
                  : t("contracts.remote.no_sms_account")
              }
              disabled={!smsAvailable}
            />
            {withoutPhone.length ? (
              <p className="text-muted-foreground text-xs">
                {t("contracts.remote.without_phone", {
                  names: withoutPhone.map((s) => s.name).join(", "),
                })}
              </p>
            ) : null}
            <p className="text-muted-foreground text-xs">
              {t("contracts.remote.disclaimer")}
            </p>
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

/**
 * Remote approval links of a document: status, masked number, expiry;
 * copy, send by SMS, revoke. Creating a link belongs to the current
 * version (the caller opens CreateLinkDialog).
 */
export function RemoteLinksTable({
  documentUuid,
  canWrite,
}: {
  documentUuid: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const query = useApprovalLinks(documentUuid);
  const send = useSendApprovalLink();
  const revoke = useRevokeApprovalLink();
  const [revoking, setRevoking] = useState<ApprovalLink | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const smsAvailable =
    useDocumentSigners(documentUuid, canWrite).data?.sms_available ?? false;

  const columns = useMemo(
    () =>
      [
        createColumn<ApprovalLink>({
          accessorKey: "signer_name",
          labelKey: "contracts.remote.columns.signer",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.signer_name}</span>
              <span className="text-muted-foreground text-xs">
                {t(
                  SIGNER_ROLE_LABEL_KEYS[row.original.signer_role] ??
                    row.original.signer_role,
                )}{" "}
                · {row.original.phone_masked}
              </span>
            </span>
          ),
        }),
        createColumn<ApprovalLink>({
          accessorKey: "status",
          labelKey: "contracts.remote.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(LINK_STATUS_LABEL_KEYS[row.original.status])}
              tone={LINK_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<ApprovalLink>({
          accessorKey: "version_number",
          labelKey: "contracts.remote.columns.version",
          enableSorting: false,
          cell: ({ row }) => `v${row.original.version_number}`,
        }),
        createColumn<ApprovalLink>({
          accessorKey: "expires_at",
          labelKey: "contracts.remote.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.approved_at
              ? t("contracts.remote.approved_at", {
                  at: datetime(row.original.approved_at, "d MMM HH:mm", locale),
                })
              : datetime(row.original.expires_at, "d MMM HH:mm", locale),
        }),
        createColumn<ApprovalLink>({
          id: "sms",
          accessorFn: (row) => row.sms_sent_count,
          labelKey: "contracts.remote.columns.sms",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("contracts.remote.sms_counts", {
              sent: row.original.sms_sent_count,
              codes: row.original.codes_sent,
            }),
        }),
        createColumn<ApprovalLink>({
          id: "actions",
          labelKey: "contracts.remote.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const l = row.original;
            if (l.status !== "pending" || !l.url) return null;
            const url = l.url;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "copy",
                    label:
                      copiedId === l.uuid
                        ? t("contracts.remote.copied")
                        : t("contracts.remote.copy"),
                    icon: copiedId === l.uuid ? Check : Copy,
                    onSelect: () =>
                      copyText(url, () => {
                        setCopiedId(l.uuid);
                        window.setTimeout(() => setCopiedId(null), 2000);
                      }),
                  },
                  ...(canWrite
                    ? [
                        {
                          id: "sms",
                          label: t("contracts.remote.send_sms"),
                          icon: MessageSquare,
                          disabled: !smsAvailable || send.isPending,
                          onSelect: () =>
                            send.mutate({ doc: documentUuid, link: l.uuid }),
                        },
                        {
                          id: "revoke",
                          label: t("contracts.remote.revoke"),
                          icon: Ban,
                          variant: "destructive" as const,
                          onSelect: () => setRevoking(l),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ApprovalLink, unknown>[],
    [canWrite, copiedId, documentUuid, locale, send, smsAvailable, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("contracts.remote.empty")}
        emptyDescription={t("contracts.remote.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-document-approval-links-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
      <ConfirmDialog
        open={Boolean(revoking)}
        title={t("contracts.remote.revoke_title")}
        description={t("contracts.remote.revoke_description", {
          name: revoking?.signer_name ?? "",
        })}
        confirmLabel={t("contracts.remote.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onCancel={() => setRevoking(null)}
        onConfirm={() => {
          if (!revoking) return;
          revoke.mutate(
            { doc: documentUuid, link: revoking.uuid },
            { onSettled: () => setRevoking(null) },
          );
        }}
      />
    </>
  );
}
