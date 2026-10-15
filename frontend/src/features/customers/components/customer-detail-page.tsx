"use client";

import {
  Building2,
  Eye,
  MoreHorizontal,
  Pencil,
  ShieldX,
  Trash2,
  User,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerActivityCard } from "@/features/customers/components/customer-activity-card";
import {
  ConsentChips,
  CustomerTypeChip,
  MaskedValue,
} from "@/features/customers/components/customer-cells";
import { CustomerConsentsTab } from "@/features/customers/components/customer-consents-tab";
import { CustomerDocumentsTab } from "@/features/customers/components/customer-documents-tab";
import { CustomerDriversTab } from "@/features/customers/components/customer-drivers-tab";
import { CustomerGeneralTab } from "@/features/customers/components/customer-general-tab";
import { CustomerLedgerCard } from "@/features/finance/components/customer-ledger-card";
import {
  CustomerRevealProvider,
  useCustomerReveal,
} from "@/features/customers/components/customer-reveal";
import {
  useAnonymizeCustomer,
  useCustomer,
  useDeleteCustomer,
} from "@/features/customers/hooks/use-customers";
import type { Customer } from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Tabs of the customer page, in display order (`?tab=` values). */
const CUSTOMER_TABS = [
  "general",
  "drivers",
  "consents",
  "documents",
  "rentals",
  "account",
  "activity",
] as const;

type CustomerTab = (typeof CUSTOMER_TABS)[number];

const TAB_LABEL_KEYS: Record<CustomerTab, string> = {
  general: "customers.tabs.general",
  drivers: "customers.tabs.drivers",
  consents: "customers.tabs.consents",
  documents: "customers.tabs.documents",
  rentals: "customers.tabs.rentals",
  account: "customers.tabs.account",
  activity: "customers.tabs.activity",
};

function resolveTab(value: string | undefined): CustomerTab {
  return value && (CUSTOMER_TABS as readonly string[]).includes(value)
    ? (value as CustomerTab)
    : "general";
}

const ANONYMIZE_FORM_ID = "customer-anonymize-form";

/**
 * KVKK data subject request: wipes the personal data and keeps the row.
 * The user types the customer's name to confirm (irreversible).
 */
function AnonymizeDialog({
  customer,
  open,
  onOpenChange,
}: {
  customer: Customer;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const anonymize = useAnonymizeCustomer();
  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("customers.reveal.reason_short"))
          .max(500),
        confirm: z
          .string()
          .refine(
            (v) =>
              v.trim().toLocaleLowerCase("tr") ===
              customer.display_name.trim().toLocaleLowerCase("tr"),
            t("customers.anonymize.confirm_mismatch"),
          ),
      }),
    [customer.display_name, t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!anonymize.isPending) onOpenChange(next);
      }}
      title={t("customers.anonymize.title")}
      description={t("customers.anonymize.description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            variant="destructive"
            form={ANONYMIZE_FORM_ID}
            disabled={anonymize.isPending}
          >
            {t("customers.anonymize.confirm")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<{ reason: string; confirm: string }>
          id={ANONYMIZE_FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ reason: "", confirm: "" }}
          onSubmit={async (values) => {
            await anonymize.mutateAsync({
              uuid: customer.uuid,
              reason: values.reason.trim(),
            });
            onOpenChange(false);
          }}
        >
          <Alert variant="destructive">
            <ShieldX className="size-4" />
            <AlertDescription>
              {t("customers.anonymize.warning")}
            </AlertDescription>
          </Alert>
          <AppTextarea
            name="reason"
            label={t("customers.anonymize.reason")}
            placeholder={t("customers.anonymize.reason_placeholder")}
          />
          <AppInput
            name="confirm"
            label={t("customers.anonymize.confirm_label", {
              name: customer.display_name,
            })}
            autoComplete="off"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function ComingSoonTab({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <EntitySectionCard title={title}>
      <EmptyState className="py-10" title={description} />
    </EntitySectionCard>
  );
}

function RevealHeaderButton({ customer }: { customer: Customer }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { revealed, open } = useCustomerReveal();
  if (!can(permissions.tenant.customers.reveal) || customer.anonymized) {
    return null;
  }
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={Boolean(revealed)}
      onClick={open}
    >
      <Eye className="size-4" />
      {t("customers.reveal.button")}
    </Button>
  );
}

/**
 * Customer show page (§11.1): header with the type, masked identity and
 * consents, then tabs (Genel, Sürücüler, İzinler, Belgeler, Kiralamalar,
 * Cari, Aktivite). Revealed values are shared by the tabs for the reveal
 * window only.
 */
export function CustomerDetailPage({
  uuid,
  initialTab,
}: {
  uuid: string;
  initialTab?: string;
}) {
  return (
    <CustomerRevealProvider key={uuid} customerUuid={uuid}>
      <CustomerDetail uuid={uuid} initialTab={initialTab} />
    </CustomerRevealProvider>
  );
}

function CustomerDetail({
  uuid,
  initialTab,
}: {
  uuid: string;
  initialTab?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const query = useCustomer(uuid);
  const remove = useDeleteCustomer();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [anonymizeOpen, setAnonymizeOpen] = useState(false);
  const [tab, setTab] = useState<CustomerTab>(() => resolveTab(initialTab));
  const openTab = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };

  const customer = query.data;
  const title = customer
    ? customer.display_name || t("customers.anonymized_badge")
    : t("customers.detail_title");
  const canWrite = can(permissions.tenant.customers.write);
  const canAnonymize = can(permissions.tenant.customers.anonymize);

  return (
    <EntityPage
      title={title}
      description={t("customers.detail_description")}
      permission={permissions.tenant.customers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.title"),
          href: routes.tenant.customers.root(slug),
        },
        { label: title },
      ]}
      actions={
        customer ? (
          <EntityActions>
            <RevealHeaderButton customer={customer} />
            {canWrite && !customer.anonymized ? (
              <Button asChild size="sm">
                <Link href={routes.tenant.customers.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("customers.actions.edit")}
                </Link>
              </Button>
            ) : null}
            {canWrite || canAnonymize ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    size="icon"
                    variant="outline"
                    className="size-8"
                    aria-label={t("customers.actions.more")}
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canAnonymize && !customer.anonymized ? (
                    <DropdownMenuItem onSelect={() => setAnonymizeOpen(true)}>
                      <ShieldX className="size-4" />
                      {t("customers.actions.anonymize")}
                    </DropdownMenuItem>
                  ) : null}
                  {canWrite ? (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onSelect={() => setConfirmDelete(true)}
                      >
                        <Trash2 className="size-4" />
                        {t("customers.actions.delete")}
                      </DropdownMenuItem>
                    </>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("customers.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {customer ? (
        <div className="space-y-6">
          <EntityHeader
            leading={
              <div className="bg-muted/40 flex size-14 shrink-0 items-center justify-center rounded-lg border">
                {customer.customer_type === "corporate" ? (
                  <Building2 className="text-muted-foreground size-6" />
                ) : (
                  <User className="text-muted-foreground size-6" />
                )}
              </div>
            }
            title={title}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <MaskedValue value={customer.identity_masked} />
                <span>·</span>
                <MaskedValue value={customer.phone_masked} />
              </span>
            }
            badges={
              <>
                <CustomerTypeChip type={customer.customer_type} />
                {customer.anonymized ? (
                  <StatusChip
                    label={t("customers.anonymized_badge")}
                    tone="warning"
                  />
                ) : null}
              </>
            }
          >
            <div className="text-muted-foreground flex flex-wrap items-center gap-x-5 gap-y-1 text-sm">
              <span className="flex items-center gap-1.5">
                {t("customers.fields.consents")}:
                <ConsentChips channels={customer.consent_channels} />
              </span>
              <span>
                {t("customers.header.drivers", {
                  count: customer.drivers_count,
                })}
              </span>
            </div>
          </EntityHeader>

          <Tabs value={tab} onValueChange={openTab} className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1 pb-1">
              <TabsList className="h-auto w-max">
                {CUSTOMER_TABS.map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(TAB_LABEL_KEYS[value])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>

            <TabsContent value="general">
              <CustomerGeneralTab customer={customer} />
            </TabsContent>
            <TabsContent value="drivers">
              <CustomerDriversTab customer={customer} />
            </TabsContent>
            <TabsContent value="consents">
              <CustomerConsentsTab
                customerUuid={uuid}
                readOnly={customer.anonymized}
              />
            </TabsContent>
            <TabsContent value="documents">
              <CustomerDocumentsTab
                customerUuid={uuid}
                readOnly={customer.anonymized}
              />
            </TabsContent>
            <TabsContent value="rentals">
              <ComingSoonTab
                title={t("customers.tabs.rentals")}
                description={t("customers.coming.rentals")}
              />
            </TabsContent>
            <TabsContent value="account">
              <CustomerLedgerCard customerUuid={uuid} />
            </TabsContent>
            <TabsContent value="activity">
              <CustomerActivityCard uuid={uuid} />
            </TabsContent>
          </Tabs>

          <AnonymizeDialog
            customer={customer}
            open={anonymizeOpen}
            onOpenChange={setAnonymizeOpen}
          />
          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={title}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.customers.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
