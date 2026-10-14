import { CatalogImportDetailPage } from "@/features/catalog";

type PageProps = {
  params: Promise<{ uuid: string }>;
};

export default async function PlatformCatalogImportDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <CatalogImportDetailPage uuid={uuid} />;
}
