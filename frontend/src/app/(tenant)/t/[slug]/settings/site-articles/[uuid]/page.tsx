import { SiteArticleEditorPage } from "@/features/site-articles";

export default async function Page({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <SiteArticleEditorPage uuid={uuid} />;
}
