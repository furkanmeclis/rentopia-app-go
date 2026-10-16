import { KabisDetailPage } from "@/features/kabis";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <KabisDetailPage uuid={uuid} />;
}
