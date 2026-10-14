import { CatalogModelsPage } from "@/features/catalog";

type PageProps = {
  searchParams: Promise<{ brand?: string; display_brand?: string }>;
};

export default async function PlatformCatalogModelsPage({
  searchParams,
}: PageProps) {
  const { brand, display_brand } = await searchParams;
  return (
    <CatalogModelsPage
      key={`${brand ?? ""}:${display_brand ?? ""}`}
      initialBrand={brand}
      initialDisplayBrand={display_brand}
    />
  );
}
