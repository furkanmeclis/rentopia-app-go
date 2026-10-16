"use client";

import { Pencil, ToggleRight, Trash2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AgreementStatusChip,
  ValidityChip,
  ValidityRange,
} from "@/features/agreements/components/agreement-cells";
import { AgreementStatusDialog } from "@/features/agreements/components/agreement-dialogs";
import { AgreementDrawer } from "@/features/agreements/components/agreement-drawer";
import {
  AgreementBillingTab,
  AgreementPeopleTab,
  AgreementRentalsTab,
  AgreementStatementTab,
  AgreementTariffsTab,
} from "@/features/agreements/components/agreement-tabs";
import {
  AGREEMENT_TRANSITIONS,
  INVOICE_PERIOD_HINT_KEYS,
  INVOICE_PERIOD_LABEL_KEYS,
} from "@/features/agreements/constants";
import {
  useAgreement,
  useAgreementActivity,
  useDeleteAgreement,
} from "@/features/agreements/hooks/use-agreements";
import {
  ActivityFeedCard,
  useActivityLimit,
} from "@/features/extras/components/extras-shared";
import { CodeBadge } from "@/features/fleet/components/shared/list-utils";
import { moneyText } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Activity({ uuid }: { uuid: string }) {
  const { limit, more } = useActivityLimit();
  const query = useAgreementActivity(uuid, { limit, offset: 0 });
  return <ActivityFeedCard query={query} onLoadMore={more} />;
}

/**
 * Corporate agreement show page: overview (terms, payment conditions,
 * open-account exposure), tariffs, contacts and drivers, rentals, billing
 * plan and statement.
 */
export function AgreementDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useAgreement(uuid);
  const remove = useDeleteAgreement();
  const a = query.data;
  const [editOpen, setEditOpen] = useState(false);
  const [statusOpen, setStatusOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const title = a?.name ?? t("agreements.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("agreements.detail_description")}
      permission={permissions.tenant.agreements.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("agreements.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("agreements.title"),
          href: routes.tenant.agreements.root(slug),
        },
        { label: title },
      ]}
      actions={
        a ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.agreements.write}>
              {AGREEMENT_TRANSITIONS[a.status].length > 0 ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setStatusOpen(true)}
                >
                  <ToggleRight className="size-4" />
                  {t("agreements.actions.status")}
                </Button>
              ) : null}
              {a.status !== "terminated" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setEditOpen(true)}
                >
                  <Pencil className="size-4" />
                  {t("agreements.actions.edit")}
                </Button>
              ) : null}
              {a.status === "draft" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setDeleteOpen(true)}
                >
                  <Trash2 className="size-4" />
                  {t("agreements.actions.delete")}
                </Button>
              ) : null}
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("agreements.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {a ? (
        <div className="space-y-6">
          <EntityHeader
            title={a.name}
            subtitle={a.customer.display_name}
            badges={
              <>
                <CodeBadge code={a.code} />
                <AgreementStatusChip status={a.status} />
                <ValidityChip validity={a.validity} />
              </>
            }
          />
          <Tabs defaultValue="overview" className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1">
              <TabsList className="flex h-auto w-max flex-wrap sm:w-auto">
                <TabsTrigger value="overview">
                  {t("agreements.tabs.overview")}
                </TabsTrigger>
                <TabsTrigger value="tariffs">
                  {t("agreements.tabs.tariffs")}
                </TabsTrigger>
                <TabsTrigger value="people">
                  {t("agreements.tabs.people")}
                </TabsTrigger>
                <TabsTrigger value="rentals">
                  {t("agreements.tabs.rentals")}
                </TabsTrigger>
                <TabsTrigger value="billing">
                  {t("agreements.tabs.billing")}
                </TabsTrigger>
                <TabsTrigger value="statement">
                  {t("agreements.tabs.statement")}
                </TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value="overview" className="space-y-4">
              <div className="grid gap-4 lg:grid-cols-2">
                <EntitySectionCard title={t("agreements.detail.terms")}>
                  <EntityDetail
                    sections={[
                      {
                        id: "terms",
                        fields: [
                          {
                            key: "customer",
                            label: t("agreements.fields.customer"),
                            value: (
                              <span className="flex flex-col">
                                <Link
                                  className="font-medium underline-offset-2 hover:underline"
                                  href={routes.tenant.customers.detail(
                                    slug,
                                    a.customer.uuid,
                                  )}
                                >
                                  {a.customer.display_name}
                                </Link>
                                <span className="text-muted-foreground font-mono text-xs">
                                  {a.customer.identity_masked || ""}
                                </span>
                              </span>
                            ),
                          },
                          {
                            key: "validity",
                            label: t("agreements.fields.validity"),
                            value: (
                              <ValidityRange
                                from={a.valid_from}
                                to={a.valid_to}
                              />
                            ),
                          },
                          {
                            key: "branch",
                            label: t("agreements.fields.branch"),
                            value: a.branch
                              ? `${a.branch.code ?? ""} · ${a.branch.name}`
                              : t("agreements.head_office"),
                          },
                          {
                            key: "long_term",
                            label: t("agreements.detail.long_term"),
                            value: (
                              <span className="flex flex-wrap gap-1">
                                <Badge variant="outline">
                                  {a.monthly_km_allowance
                                    ? t("agreements.detail.km_month", {
                                        km: a.monthly_km_allowance.toLocaleString(
                                          locale,
                                        ),
                                      })
                                    : t("agreements.detail.km_unlimited")}
                                </Badge>
                                {a.maintenance_interval_days ? (
                                  <Badge variant="outline">
                                    {t("agreements.detail.maintenance_days", {
                                      days: a.maintenance_interval_days,
                                    })}
                                  </Badge>
                                ) : null}
                                {a.maintenance_interval_km ? (
                                  <Badge variant="outline">
                                    {t("agreements.detail.maintenance_km", {
                                      km: a.maintenance_interval_km.toLocaleString(
                                        locale,
                                      ),
                                    })}
                                  </Badge>
                                ) : null}
                              </span>
                            ),
                          },
                          {
                            key: "notes",
                            label: t("agreements.fields.notes"),
                            value: a.notes || "—",
                          },
                          ...(a.terminated_at
                            ? [
                                {
                                  key: "terminated",
                                  label: t("agreements.detail.terminated"),
                                  value: `${datetime(a.terminated_at, undefined, locale)} · ${a.termination_reason}`,
                                },
                              ]
                            : []),
                        ],
                      },
                    ]}
                  />
                </EntitySectionCard>
                <EntitySectionCard title={t("agreements.detail.payment")}>
                  <EntityDetail
                    sections={[
                      {
                        id: "payment",
                        fields: [
                          {
                            key: "invoice_period",
                            label: t("agreements.fields.invoice_period"),
                            value: (
                              <span className="flex flex-col">
                                <span>
                                  {t(
                                    INVOICE_PERIOD_LABEL_KEYS[a.invoice_period],
                                  )}
                                </span>
                                <span className="text-muted-foreground text-xs">
                                  {t(
                                    INVOICE_PERIOD_HINT_KEYS[a.invoice_period],
                                  )}
                                </span>
                              </span>
                            ),
                          },
                          {
                            key: "term",
                            label: t("agreements.fields.payment_term_days"),
                            value: (
                              <span className="flex flex-col">
                                <span>
                                  {t("agreements.days", {
                                    count: a.payment_term_days,
                                  })}
                                </span>
                                {a.account ? (
                                  <span className="text-muted-foreground text-xs">
                                    {t("agreements.detail.account_term", {
                                      code: a.account.code,
                                      days: a.account.payment_term_days,
                                    })}
                                  </span>
                                ) : null}
                              </span>
                            ),
                          },
                          {
                            key: "open_account",
                            label: t("agreements.fields.open_account"),
                            value: a.open_account
                              ? t("agreements.detail.open_account_limit", {
                                  limit: moneyText(a.credit_limit, locale),
                                })
                              : t("agreements.detail.prepayment_rules"),
                          },
                          {
                            key: "coupon",
                            label: t("agreements.fields.coupon_eligible"),
                            value: a.coupon_eligible
                              ? t("agreements.detail.coupons_corporate")
                              : t("agreements.detail.coupons_none"),
                          },
                        ],
                      },
                    ]}
                  />
                  {a.exposure ? (
                    <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
                      {(
                        [
                          [
                            "agreements.exposure.credit_limit",
                            a.exposure.credit_limit,
                          ],
                          ["agreements.exposure.balance", a.exposure.balance],
                          ["agreements.exposure.pending", a.exposure.pending],
                          [
                            "agreements.exposure.available",
                            a.exposure.available,
                          ],
                        ] as const
                      ).map(([key, value]) => (
                        <div key={key} className="rounded-lg border p-2">
                          <p className="text-muted-foreground text-xs">
                            {t(key)}
                          </p>
                          <p
                            className={
                              key === "agreements.exposure.available" &&
                              Number(value) < 0
                                ? "text-destructive font-semibold tabular-nums"
                                : "font-semibold tabular-nums"
                            }
                          >
                            {moneyText(value, locale)}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </EntitySectionCard>
              </div>
              <Activity uuid={a.uuid} />
            </TabsContent>
            <TabsContent value="tariffs">
              <AgreementTariffsTab agreement={a} />
            </TabsContent>
            <TabsContent value="people">
              <AgreementPeopleTab agreement={a} />
            </TabsContent>
            <TabsContent value="rentals">
              <AgreementRentalsTab agreement={a} />
            </TabsContent>
            <TabsContent value="billing">
              <AgreementBillingTab agreement={a} />
            </TabsContent>
            <TabsContent value="statement">
              <AgreementStatementTab agreement={a} />
            </TabsContent>
          </Tabs>
          <AgreementDrawer
            open={editOpen}
            agreement={a}
            onOpenChange={setEditOpen}
          />
          {statusOpen ? (
            <AgreementStatusDialog
              agreement={a}
              open={statusOpen}
              onOpenChange={setStatusOpen}
            />
          ) : null}
          <EntityDeleteDialog
            open={deleteOpen}
            entityLabel={`${a.code} · ${a.name}`}
            isPending={remove.isPending}
            onCancel={() => setDeleteOpen(false)}
            onConfirm={() =>
              remove.mutate(a.uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.agreements.root(slug)),
                onSettled: () => setDeleteOpen(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
