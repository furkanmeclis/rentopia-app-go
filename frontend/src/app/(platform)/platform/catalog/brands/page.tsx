import { CatalogBrandsPage } from "@/features/catalog";

type PageProps = {
  searchParams: Promise<{ view?: string; display_brand?: string }>;
};

export default async function PlatformCatalogBrandsPage({
  searchParams,
}: PageProps) {
  const { view, display_brand } = await searchParams;
  return (
    <CatalogBrandsPage
      key={`${view ?? ""}:${display_brand ?? ""}`}
      initialView={view === "display" ? "display" : "tsb"}
      initialDisplayBrand={display_brand}
    />
  );
}
