"use client";

import { Info } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppSwitch, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import {
  useCampaignStorefront,
  useUpdateCampaignStorefront,
} from "@/features/campaigns/hooks/use-campaigns";
import type {
  Campaign,
  CampaignStorefront,
  CampaignStorefrontPatch,
} from "@/features/campaigns/types";
import { MediaField } from "@/features/site/components/site-media";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

/** API limits of the storefront texts (campaigns/usecase/storefront.go). */
const LIMITS = { title: 160, summary: 1000, cta: 80 } as const;
const SORT_MIN = -100000;
const SORT_MAX = 100000;
const FORM_ID = "campaign-storefront-form";

type Values = {
  show_on_site: boolean;
  site_sort: string;
  site_title: { tr: string; en: string };
  site_summary: { tr: string; en: string };
  site_cta_label: { tr: string; en: string };
  site_media_uuid: string;
};

function defaults(s: CampaignStorefront): Values {
  const pair = (v: { tr?: string; en?: string } | undefined) => ({
    tr: v?.tr ?? "",
    en: v?.en ?? "",
  });
  return {
    show_on_site: s.show_on_site,
    site_sort: String(s.site_sort ?? 0),
    site_title: pair(s.site_title),
    site_summary: pair(s.site_summary),
    site_cta_label: pair(s.site_cta_label),
    site_media_uuid: s.site_media_uuid ?? "",
  };
}

function body(v: Values): CampaignStorefrontPatch {
  const pair = (p: { tr: string; en: string }) => ({
    tr: p.tr.trim(),
    en: p.en.trim(),
  });
  return {
    show_on_site: v.show_on_site,
    site_sort: Number(v.site_sort || 0),
    site_title: pair(v.site_title),
    site_summary: pair(v.site_summary),
    site_cta_label: pair(v.site_cta_label),
    site_media_uuid: v.site_media_uuid || null,
  };
}

/** A titled group of fields inside the card (no nested card borders). */
function Group({
  title,
  description,
  columns = 2,
  children,
}: {
  title: string;
  description?: string;
  columns?: 1 | 2;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-semibold tracking-tight">{title}</h3>
        {description ? (
          <p className="text-muted-foreground text-xs">{description}</p>
        ) : null}
      </div>
      <div
        className={columns === 2 ? "grid gap-4 sm:grid-cols-2" : "grid gap-4"}
      >
        {children}
      </div>
    </section>
  );
}

/**
 * Why a campaign marked for the website would still not be listed there
 * (the API shows only active campaigns sold on the web channel).
 */
function visibilityNotes(campaign: Campaign, t: (k: string) => string) {
  const notes: string[] = [];
  if (campaign.state !== "active")
    notes.push(t("campaigns.storefront.note_inactive"));
  if (campaign.channels != null && !campaign.channels.includes("web"))
    notes.push(t("campaigns.storefront.note_channel"));
  return notes;
}

/**
 * "Sitede vitrin": the campaign's card on the branded site (TEC-784):
 * visibility, order, texts in Turkish / English and a library image. Dates
 * and the coupon code always come from the campaign itself.
 */
export function CampaignStorefrontCard({ campaign }: { campaign: Campaign }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.campaigns.write);
  const canSiteMedia = can(permissions.tenant.site.read);
  const canUpload = can(permissions.tenant.site.manage);
  const query = useCampaignStorefront(campaign.uuid);
  const update = useUpdateCampaignStorefront(campaign.uuid);
  const schema = useMemo(() => {
    const text = (max: number) =>
      z.string().trim().max(max, t("campaigns.storefront.too_long", { max }));
    const pair = (max: number) => z.object({ tr: text(max), en: text(max) });
    return z
      .object({
        show_on_site: z.boolean(),
        site_sort: z
          .string()
          .trim()
          .refine(
            (v) =>
              v === "" ||
              (/^-?\d{1,6}$/.test(v) &&
                Number(v) >= SORT_MIN &&
                Number(v) <= SORT_MAX),
            t("campaigns.storefront.sort_invalid"),
          ),
        site_title: pair(LIMITS.title),
        site_summary: pair(LIMITS.summary),
        site_cta_label: pair(LIMITS.cta),
        site_media_uuid: z.string(),
      })
      .refine(
        (v) =>
          !v.show_on_site ||
          v.site_title.tr.trim() !== "" ||
          v.site_title.en.trim() !== "",
        {
          path: ["site_title", "tr"],
          message: t("campaigns.storefront.title_required"),
        },
      );
  }, [t]) as unknown as z.ZodType<Values>;

  const data = query.data;
  const notes = visibilityNotes(campaign, t);
  const codeShown =
    campaign.campaign_type === "coupon" &&
    Boolean(campaign.code) &&
    campaign.customer_uuids == null;
  const live = Boolean(data?.show_on_site) && notes.length === 0;

  return (
    <EntitySectionCard
      title={t("campaigns.storefront.title")}
      actions={
        data ? (
          <StatusChip
            label={t(
              live
                ? "campaigns.storefront.status_live"
                : data.show_on_site
                  ? "campaigns.storefront.status_waiting"
                  : "campaigns.storefront.status_hidden",
            )}
            tone={live ? "success" : data.show_on_site ? "warning" : "default"}
          />
        ) : null
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("campaigns.storefront.help")}
      </p>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("campaigns.storefront.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {data ? (
        <AppForm<Values>
          key={JSON.stringify(data)}
          id={FORM_ID}
          className="space-y-6"
          schema={schema}
          defaultValues={defaults(data)}
          onSubmit={async (values) => {
            await update.mutateAsync(body(values));
          }}
        >
          {data.show_on_site && notes.length ? (
            <Alert>
              <Info className="size-4" />
              <AlertDescription>
                <ul className="list-disc space-y-1 pl-4">
                  {notes.map((n) => (
                    <li key={n}>{n}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ) : null}
          <Group title={t("campaigns.storefront.section_visibility")}>
            <AppSwitch
              name="show_on_site"
              label={t("campaigns.storefront.show_on_site")}
              description={t("campaigns.storefront.show_on_site_hint")}
              className="sm:col-span-2"
              disabled={!canWrite}
            />
            <AppInput
              name="site_sort"
              type="number"
              inputMode="numeric"
              step={1}
              min={SORT_MIN}
              max={SORT_MAX}
              label={t("campaigns.storefront.site_sort")}
              description={t("campaigns.storefront.site_sort_hint")}
              disabled={!canWrite}
            />
            <div className="text-muted-foreground space-y-1 text-sm">
              <p className="text-foreground font-medium">
                {t("campaigns.storefront.code_title")}
              </p>
              <p>
                {codeShown
                  ? t("campaigns.storefront.code_shown", {
                      code: campaign.code ?? "",
                    })
                  : t("campaigns.storefront.code_hidden")}
              </p>
            </div>
          </Group>
          <Separator />
          <Group
            title={t("campaigns.storefront.section_texts")}
            description={t("campaigns.storefront.section_texts_hint")}
          >
            <AppInput
              name="site_title.tr"
              label={t("campaigns.storefront.site_title_tr")}
              maxLength={LIMITS.title}
              disabled={!canWrite}
            />
            <AppInput
              name="site_title.en"
              label={t("campaigns.storefront.site_title_en")}
              maxLength={LIMITS.title}
              disabled={!canWrite}
            />
            <AppTextarea
              name="site_summary.tr"
              label={t("campaigns.storefront.site_summary_tr")}
              rows={3}
              maxLength={LIMITS.summary}
              disabled={!canWrite}
            />
            <AppTextarea
              name="site_summary.en"
              label={t("campaigns.storefront.site_summary_en")}
              rows={3}
              maxLength={LIMITS.summary}
              disabled={!canWrite}
            />
            <AppInput
              name="site_cta_label.tr"
              label={t("campaigns.storefront.site_cta_tr")}
              description={t("campaigns.storefront.site_cta_hint")}
              maxLength={LIMITS.cta}
              disabled={!canWrite}
            />
            <AppInput
              name="site_cta_label.en"
              label={t("campaigns.storefront.site_cta_en")}
              description={t("campaigns.storefront.site_cta_hint")}
              maxLength={LIMITS.cta}
              disabled={!canWrite}
            />
          </Group>
          <Separator />
          <Group title={t("campaigns.storefront.section_image")} columns={1}>
            {canSiteMedia ? (
              <MediaField
                name="site_media_uuid"
                label={t("campaigns.storefront.image")}
                description={t("campaigns.storefront.image_hint")}
                disabled={!canWrite}
                canUpload={canUpload}
              />
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("campaigns.storefront.image_no_access")}
              </p>
            )}
          </Group>
          {canWrite ? (
            <div className="flex justify-end">
              <Button type="submit" disabled={update.isPending}>
                {t("campaigns.storefront.save")}
              </Button>
            </div>
          ) : null}
        </AppForm>
      ) : null}
    </EntitySectionCard>
  );
}
