import { ProductDetailPage } from "@/features/fleet";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <ProductDetailPage uuid={uuid} />;
}
