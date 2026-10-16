import { AgreementDetailPage } from "@/features/agreements";

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { uuid } = await params;
  return <AgreementDetailPage uuid={uuid} />;
}
