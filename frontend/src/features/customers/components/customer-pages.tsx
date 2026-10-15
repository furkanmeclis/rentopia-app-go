"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerForm } from "@/features/customers/components/customer-form";
import {
  useCreateCustomer,
  useCustomer,
  useUpdateCustomer,
} from "@/features/customers/hooks/use-customers";
import {
  customerFormDefaults,
  customerFormToRequest,
} from "@/features/customers/schemas/customer-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("customers.forbidden")}
    />
  );
}

export function CustomerCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateCustomer();

  return (
    <EntityPage
      title={t("customers.create_title")}
      description={t("customers.create_description")}
      permission={permissions.tenant.customers.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.title"),
          href: routes.tenant.customers.root(slug),
        },
        { label: t("customers.create_title") },
      ]}
    >
      <EntityForm>
        <CustomerForm
          mode="create"
          defaultValues={customerFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.customers.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              customerFormToRequest(values, "create"),
            );
            router.push(routes.tenant.customers.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

export function CustomerEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useCustomer(uuid);
  const update = useUpdateCustomer();
  const customer = query.data;

  return (
    <EntityPage
      title={
        customer
          ? t("customers.edit_title_with_name", {
              name: customer.display_name,
            })
          : t("customers.edit_title")
      }
      description={t("customers.edit_description")}
      permission={permissions.tenant.customers.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.title"),
          href: routes.tenant.customers.root(slug),
        },
        {
          label: customer?.display_name || t("customers.detail_title"),
          href: routes.tenant.customers.detail(slug, uuid),
        },
        { label: t("customers.edit_title") },
      ]}
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
      {customer?.anonymized ? (
        <ErrorState
          title={t("customers.anonymized_badge")}
          description={t("customers.anonymized_readonly")}
        />
      ) : null}
      {customer && !customer.anonymized ? (
        <EntityForm>
          <CustomerForm
            key={customer.updated_at}
            mode="edit"
            customer={customer}
            defaultValues={customerFormDefaults(customer)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.customers.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: customerFormToRequest(values, "edit", customer),
              });
              router.push(routes.tenant.customers.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
