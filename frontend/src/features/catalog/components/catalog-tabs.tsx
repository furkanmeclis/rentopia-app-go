"use client";

import Link from "next/link";

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export type CatalogTab =
  "brands" | "models" | "types" | "unresolved" | "imports" | "logos";

const TABS: { id: CatalogTab; href: string; labelKey: string }[] = [
  {
    id: "brands",
    href: routes.platform.catalog.brands,
    labelKey: "catalog.tabs.brands",
  },
  {
    id: "models",
    href: routes.platform.catalog.models,
    labelKey: "catalog.tabs.models",
  },
  {
    id: "types",
    href: routes.platform.catalog.types,
    labelKey: "catalog.tabs.types",
  },
  {
    id: "unresolved",
    href: routes.platform.catalog.unresolved,
    labelKey: "catalog.tabs.unresolved",
  },
  {
    id: "imports",
    href: routes.platform.catalog.imports,
    labelKey: "catalog.tabs.imports",
  },
  {
    id: "logos",
    href: routes.platform.catalog.logos,
    labelKey: "catalog.tabs.logos",
  },
];

/** Route-backed tabs shared by the catalog list pages. */
export function CatalogTabs({ active }: { active: CatalogTab }) {
  const { t } = useLocale();
  return (
    <Tabs value={active} className="mb-4">
      <TabsList>
        {TABS.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id} asChild>
            <Link href={tab.href}>{t(tab.labelKey)}</Link>
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}

export function useCatalogBreadcrumbs(current: string) {
  const { t } = useLocale();
  return [
    { label: t("layout.breadcrumb_home"), href: routes.platform.home },
    {
      label: t("catalog.title"),
      href: routes.platform.catalog.root,
    },
    { label: current },
  ];
}
