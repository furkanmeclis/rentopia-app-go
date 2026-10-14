import { CatalogModelsPage } from "@/features/catalog";

type PageProps = {
  searchParams: Promise<{ brand?: string }>;
};

export default async function PlatformCatalogModelsPage({
  searchParams,
}: PageProps) {
  const { brand } = await searchParams;
  return <CatalogModelsPage key={brand ?? ""} initialBrand={brand} />;
}
