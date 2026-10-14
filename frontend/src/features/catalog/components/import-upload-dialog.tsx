"use client";

import { FileSpreadsheet } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { useUploadImport } from "@/features/catalog/hooks/use-catalog";
import { useLocale } from "@/providers/locale-provider";

const PERIOD_RE = /^\d{4}-(0[1-9]|1[0-2])$/;

/** Upload a TSB kasko list (.xlsx); opens the import page for the dry run. */
export function ImportUploadDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const upload = useUploadImport();
  const [file, setFile] = useState<File | null>(null);
  const [period, setPeriod] = useState("");
  const periodInvalid = period !== "" && !PERIOD_RE.test(period);

  const close = (next: boolean) => {
    if (upload.isPending) return;
    if (!next) {
      setFile(null);
      setPeriod("");
    }
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("catalog.imports.upload_title")}
      description={t("catalog.imports.upload_description")}
    >
      <form
        className="space-y-4"
        onSubmit={async (event) => {
          event.preventDefault();
          if (!file || periodInvalid) return;
          const job = await upload.mutateAsync({
            file,
            period: period || undefined,
          });
          close(false);
          router.push(routes.platform.catalog.importDetail(job.uuid));
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="catalog-import-file">
            {t("catalog.imports.file")}
          </Label>
          <Input
            id="catalog-import-file"
            type="file"
            accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
          {file ? (
            <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <FileSpreadsheet className="size-3.5" />
              {file.name} · {(file.size / 1024 / 1024).toFixed(1)} MB
            </p>
          ) : null}
        </div>
        <div className="space-y-2">
          <Label htmlFor="catalog-import-period">
            {t("catalog.imports.period_override")}
          </Label>
          <Input
            id="catalog-import-period"
            placeholder="2026-10"
            value={period}
            aria-invalid={periodInvalid}
            onChange={(event) => setPeriod(event.target.value.trim())}
          />
          <p
            className={
              periodInvalid
                ? "text-destructive text-xs"
                : "text-muted-foreground text-xs"
            }
          >
            {periodInvalid
              ? t("catalog.validation.period")
              : t("catalog.imports.period_hint")}
          </p>
        </div>
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={upload.isPending}
            onClick={() => close(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            disabled={!file || periodInvalid || upload.isPending}
          >
            {upload.isPending
              ? t("catalog.imports.uploading")
              : t("catalog.imports.upload_submit")}
          </Button>
        </div>
      </form>
    </AppDialog>
  );
}
