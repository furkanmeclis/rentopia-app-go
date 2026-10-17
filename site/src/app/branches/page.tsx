import { MapPin, Phone } from "lucide-react";
import type { Metadata } from "next";

import { Container, PageTitle } from "@/components/site/text";
import { Alert, Card, CardTitle } from "@/components/ui/card";
import { loadLocations } from "@/lib/catalog";
import { getCtx } from "@/lib/server";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("branches.title") };
}

export default async function BranchesPage() {
  const { t, site } = await getCtx();
  const locations = site.booking_enabled ? await loadLocations() : null;
  return (
    <Container className="space-y-6 py-10">
      <PageTitle>{t("branches.title")}</PageTitle>
      {!locations || locations.length === 0 ? (
        <Alert>{t("branches.empty")}</Alert>
      ) : (
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {locations.map((l) => (
            <li key={l.uuid} className="flex">
              <Card className="w-full">
                <CardTitle>{l.name}</CardTitle>
                <div className="text-muted-foreground space-y-2 text-sm">
                  {l.address || l.city ? (
                    <p className="flex gap-2">
                      <MapPin className="mt-0.5 size-4 shrink-0" aria-hidden />
                      <span>
                        {[l.address, l.district, l.city]
                          .filter(Boolean)
                          .join(", ")}
                      </span>
                    </p>
                  ) : null}
                  {l.phone ? (
                    <p className="flex gap-2">
                      <Phone className="mt-0.5 size-4 shrink-0" aria-hidden />
                      <a
                        href={`tel:${l.phone.replace(/[^+0-9]/g, "")}`}
                        className="hover:underline"
                      >
                        {l.phone}
                      </a>
                    </p>
                  ) : null}
                </div>
                {l.points.length ? (
                  <div className="space-y-1 text-sm">
                    <p className="font-medium">{t("branches.points")}</p>
                    <ul className="text-muted-foreground list-inside list-disc">
                      {l.points.map((p) => (
                        <li key={p.uuid}>{p.name}</li>
                      ))}
                    </ul>
                  </div>
                ) : null}
                {l.latitude != null && l.longitude != null ? (
                  <a
                    href={`https://www.openstreetmap.org/?mlat=${l.latitude}&mlon=${l.longitude}#map=16/${l.latitude}/${l.longitude}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary text-sm underline-offset-4 hover:underline"
                  >
                    {t("branches.map")}
                  </a>
                ) : null}
              </Card>
            </li>
          ))}
        </ul>
      )}
    </Container>
  );
}
