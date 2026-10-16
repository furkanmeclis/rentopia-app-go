import { CollectionDetailPage } from "@/features/finance";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <CollectionDetailPage uuid={uuid} />;
}
