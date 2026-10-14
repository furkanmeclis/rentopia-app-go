import { CatalogTypesPage } from "@/features/catalog";

type PageProps = {
  searchParams: Promise<{
    brand?: string;
    model?: string;
    unresolved?: string;
  }>;
};

export default async function PlatformCatalogTypesPage({
  searchParams,
}: PageProps) {
  const { brand, model, unresolved } = await searchParams;
  return (
    <CatalogTypesPage
      key={`${brand ?? ""}:${model ?? ""}:${unresolved ?? ""}`}
      initialBrand={brand}
      initialModel={model}
      initialUnresolved={unresolved === "true"}
    />
  );
}
