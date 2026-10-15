import { ExtraDetailPage } from "@/features/extras";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <ExtraDetailPage uuid={uuid} />;
}
