"use client";

import { Mail, Phone } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityDetail, EntityDrawer } from "@/components/entity";
import { AppForm, AppSelect, AppTextarea } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  KIND_LABEL_KEYS,
  LEAD_STATUSES,
  NOTE_MAX,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/site-leads/constants";
import {
  useSiteLead,
  useUpdateSiteLead,
} from "@/features/site-leads/hooks/use-site-leads";
import type { SiteLead, SiteLeadStatus } from "@/features/site-leads/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "site-lead-form";

type Values = { status: SiteLeadStatus; note: string };

/** "—" for empty values (leads only carry the fields of their form). */
function text(value: string | number | null | undefined) {
  return value === null || value === undefined || value === ""
    ? "—"
    : String(value);
}

/**
 * One website enquiry: every submitted field, consents and source, plus
 * the staff handling (status and an internal note; `manage` only).
 * Everything is plain text; nothing the visitor typed is rendered as HTML.
 */
export function SiteLeadDrawer({
  lead: row,
  onOpenChange,
}: {
  lead: SiteLead | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.siteLeads.manage);
  const detail = useSiteLead(row?.uuid ?? null);
  const update = useUpdateSiteLead();
  const lead = detail.data ?? row;
  const schema = useMemo(
    () =>
      z.object({
        status: z.enum(LEAD_STATUSES as [SiteLeadStatus, ...SiteLeadStatus[]]),
        note: z
          .string()
          .max(
            NOTE_MAX,
            t("site_leads.validation.note_max", { max: NOTE_MAX }),
          ),
      }),
    [t],
  ) as unknown as z.ZodType<Values>;

  const when = (v: string | null | undefined) =>
    v ? datetime(v, undefined, locale) : "—";
  const yesNo = (v: boolean) =>
    v ? t("site_leads.detail.yes") : t("site_leads.detail.no");

  return (
    <EntityDrawer
      open={Boolean(row)}
      onOpenChange={onOpenChange}
      title={lead ? lead.name || lead.email : t("site_leads.detail.title")}
      description={
        lead
          ? `${t(KIND_LABEL_KEYS[lead.kind])} · ${when(lead.created_at)}`
          : undefined
      }
      size="lg"
      footer={
        canManage && lead ? (
          <div className="flex w-full justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("site_leads.actions.save")}
            </Button>
          </div>
        ) : undefined
      }
    >
      {detail.isError && !lead ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("site_leads.load_failed")}
          onRetry={() => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {lead ? (
        <div className="space-y-6 px-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{t(KIND_LABEL_KEYS[lead.kind])}</Badge>
            <StatusChip
              label={t(STATUS_LABEL_KEYS[lead.status])}
              tone={STATUS_TONES[lead.status]}
            />
            {lead.consent_marketing ? (
              <Badge variant="secondary">
                {t("site_leads.detail.marketing_badge")}
              </Badge>
            ) : null}
          </div>
          <EntityDetail
            sections={[
              {
                id: "contact",
                title: t("site_leads.detail.section_contact"),
                fields: [
                  {
                    key: "name",
                    label: t("site_leads.fields.name"),
                    value: text(lead.name),
                  },
                  {
                    key: "email",
                    label: t("site_leads.fields.email"),
                    value: lead.email ? (
                      <a
                        href={`mailto:${lead.email}`}
                        className="inline-flex items-center gap-1 break-all underline-offset-4 hover:underline"
                      >
                        <Mail className="size-3.5 shrink-0" />
                        {lead.email}
                      </a>
                    ) : (
                      "—"
                    ),
                  },
                  {
                    key: "phone",
                    label: t("site_leads.fields.phone"),
                    value: lead.phone ? (
                      <a
                        href={`tel:${lead.phone.replace(/[^+0-9]/g, "")}`}
                        className="inline-flex items-center gap-1 underline-offset-4 hover:underline"
                      >
                        <Phone className="size-3.5 shrink-0" />
                        {lead.phone}
                      </a>
                    ) : (
                      "—"
                    ),
                  },
                  ...(lead.kind === "corporate"
                    ? [
                        {
                          key: "company",
                          label: t("site_leads.fields.company"),
                          value: text(lead.company),
                        },
                        {
                          key: "fleet_size",
                          label: t("site_leads.fields.fleet_size"),
                          value: text(lead.fleet_size),
                        },
                      ]
                    : []),
                  ...(lead.kind === "franchise"
                    ? [
                        {
                          key: "city",
                          label: t("site_leads.fields.city"),
                          value: text(lead.city),
                        },
                      ]
                    : []),
                ],
              },
              ...(lead.kind !== "newsletter"
                ? [
                    {
                      id: "message",
                      title: t("site_leads.fields.message"),
                      children: (
                        <p className="text-sm break-words whitespace-pre-wrap">
                          {lead.message || "—"}
                        </p>
                      ),
                    },
                  ]
                : []),
              {
                id: "consent",
                title: t("site_leads.detail.section_consent"),
                fields: [
                  {
                    key: "consent_kvkk",
                    label: t("site_leads.fields.consent_kvkk"),
                    value: yesNo(lead.consent_kvkk),
                  },
                  {
                    key: "consent_marketing",
                    label: t("site_leads.fields.consent_marketing"),
                    value: yesNo(lead.consent_marketing),
                  },
                ],
              },
              {
                id: "source",
                title: t("site_leads.detail.section_source"),
                fields: [
                  {
                    key: "source_host",
                    label: t("site_leads.fields.source_host"),
                    value: (
                      <code className="text-xs break-all">
                        {text(lead.source_host)}
                      </code>
                    ),
                  },
                  {
                    key: "locale",
                    label: t("site_leads.fields.locale"),
                    value:
                      lead.locale === "en"
                        ? t("site_leads.locales.en")
                        : t("site_leads.locales.tr"),
                  },
                  {
                    key: "created_at",
                    label: t("site_leads.fields.created_at"),
                    value: when(lead.created_at),
                  },
                  {
                    key: "handled_at",
                    label: t("site_leads.fields.handled_at"),
                    value: when(lead.handled_at),
                  },
                ],
              },
            ]}
          />
          {canManage ? (
            <AppForm<Values>
              key={`${lead.uuid}-${lead.updated_at}`}
              id={FORM_ID}
              className="space-y-4 border-t pt-6"
              schema={schema}
              defaultValues={{ status: lead.status, note: lead.note }}
              onSubmit={async (values) => {
                await update.mutateAsync({
                  uuid: lead.uuid,
                  body: { status: values.status, note: values.note.trim() },
                });
              }}
            >
              <h3 className="text-sm font-semibold tracking-tight">
                {t("site_leads.detail.section_handling")}
              </h3>
              <AppSelect
                name="status"
                label={t("site_leads.fields.status")}
                options={LEAD_STATUSES.map((s) => ({
                  value: s,
                  label: t(STATUS_LABEL_KEYS[s]),
                }))}
              />
              <AppTextarea
                name="note"
                label={t("site_leads.fields.note")}
                description={t("site_leads.detail.note_hint")}
                rows={4}
                maxLength={NOTE_MAX}
              />
            </AppForm>
          ) : (
            <EntityDetail
              sections={[
                {
                  id: "handling",
                  title: t("site_leads.detail.section_handling"),
                  fields: [
                    {
                      key: "status",
                      label: t("site_leads.fields.status"),
                      value: t(STATUS_LABEL_KEYS[lead.status]),
                    },
                    {
                      key: "note",
                      label: t("site_leads.fields.note"),
                      value: (
                        <span className="whitespace-pre-wrap">
                          {text(lead.note)}
                        </span>
                      ),
                    },
                  ],
                },
              ]}
            />
          )}
        </div>
      ) : null}
    </EntityDrawer>
  );
}
