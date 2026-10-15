import { TollPassDetailPage } from "@/features/tolls";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <TollPassDetailPage uuid={uuid} />;
}
