"use client";

import { CheckCircle2, Plus, RotateCcw, Save, X } from "lucide-react";
import Link from "next/link";
import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
} from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityPage,
  EntitySectionCard,
  EntityToolbar,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io";
import {
  IssueList,
  issuesFromError,
} from "@/features/pricing/components/pricing-shared";
import {
  BRANCH_SCOPES,
  CHANNEL_SCOPES,
  DEFAULT_BANDS,
  PLAN_SCOPES,
  RATE_PLANS_RESOURCE,
  SCOPE_HINT_KEYS,
  SCOPE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/pricing/constants";
import {
  useActivateRatePlanMatrix,
  useRatePlanMatrix,
  useRatePlansMeta,
  useSaveRatePlanMatrix,
} from "@/features/pricing/hooks/use-pricing";
import {
  useBranchOptions,
  useChannelOptions,
} from "@/features/pricing/hooks/use-pricing-options";
import {
  bandLabel,
  isAmount,
  normalizeAmount,
} from "@/features/pricing/lib/format";
import type {
  MatrixQuery,
  RatePlanMatrix,
  RatePlanMatrixBand,
  RatePlanScope,
} from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Grid = Record<string, string[]>;

function bandKey(b: RatePlanMatrixBand) {
  return `${b.min_days}-${b.max_days ?? ""}`;
}

function gridFrom(m: RatePlanMatrix): Grid {
  const out: Grid = {};
  for (const row of m.rows) {
    out[row.rental_product.uuid] = row.prices.map((p) => p ?? "");
  }
  return out;
}

function sameNumber(a: string, b: string) {
  if (a.trim() === "" || b.trim() === "") return a.trim() === b.trim();
  return Number(normalizeAmount(a)) === Number(normalizeAmount(b));
}

/**
 * Product × band price matrix ("Fiyat matrisi") of one scope target. A grid
 * of shadcn inputs with spreadsheet keyboard navigation (arrows, Enter) and
 * multi-cell paste from Excel. Saving writes drafts; activation is separate
 * and all-or-nothing.
 */
export function RateMatrixPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.pricing.write);
  const branches = useBranchOptions();
  const channels = useChannelOptions();
  const [scope, setScope] =
    useState<Exclude<RatePlanScope, "agreement">>("common");
  const [branch, setBranch] = useState("");
  const [channel, setChannel] = useState("");
  const [priority, setPriority] = useState("0");
  const query: MatrixQuery = useMemo(
    () => ({
      scope,
      branch: BRANCH_SCOPES.includes(scope) && branch ? branch : undefined,
      channel: CHANNEL_SCOPES.includes(scope) && channel ? channel : undefined,
      priority: Number(priority) || 0,
    }),
    [branch, channel, priority, scope],
  );
  const targetReady =
    (!BRANCH_SCOPES.includes(scope) || Boolean(branch)) &&
    (!CHANNEL_SCOPES.includes(scope) || Boolean(channel)) &&
    /^\d{1,4}$/.test(priority);
  const matrixQuery = useRatePlanMatrix(query, targetReady);
  const metaQuery = useRatePlansMeta();
  const save = useSaveRatePlanMatrix();
  const activate = useActivateRatePlanMatrix();
  const matrix = matrixQuery.data;

  const [bands, setBands] = useState<RatePlanMatrixBand[]>([]);
  const [grid, setGrid] = useState<Grid>({});
  const [baseline, setBaseline] = useState<Grid>({});
  const [addOpen, setAddOpen] = useState(false);
  const [newBand, setNewBand] = useState({ min: "", max: "" });
  const [activateOpen, setActivateOpen] = useState(false);
  const inputs = useRef(new Map<string, HTMLInputElement>());

  // Reset the local grid whenever the server matrix changes (state adjusted
  // during render, see react.dev "you might not need an effect").
  const [syncedWith, setSyncedWith] = useState<RatePlanMatrix | null>(null);
  if (matrix && matrix !== syncedWith) {
    const initial = matrix.bands.length
      ? matrix.bands
      : DEFAULT_BANDS.map((b) => ({
          min_days: b.min_days,
          max_days: b.max_days,
        }));
    const g = gridFrom(matrix);
    if (!matrix.bands.length) {
      for (const key of Object.keys(g)) g[key] = initial.map(() => "");
    }
    setSyncedWith(matrix);
    setBands(initial);
    setGrid(g);
    setBaseline(g);
  }

  const rows = useMemo(() => matrix?.rows ?? [], [matrix]);
  const bandsChanged =
    matrix != null &&
    matrix.bands.length > 0 &&
    bands.map(bandKey).join("|") !== matrix.bands.map(bandKey).join("|");
  const changedCells = useMemo(() => {
    const out = new Set<string>();
    if (!matrix) return out;
    const serverIndex = new Map(matrix.bands.map((b, i) => [bandKey(b), i]));
    for (const row of rows) {
      const id = row.rental_product.uuid;
      bands.forEach((b, c) => {
        const si = serverIndex.get(bandKey(b));
        const before = si == null ? "" : (baseline[id]?.[si] ?? "");
        const now = grid[id]?.[c] ?? "";
        if (!sameNumber(before, now)) out.add(`${id}:${c}`);
      });
    }
    return out;
  }, [bands, baseline, grid, matrix, rows]);
  const invalidCells = useMemo(() => {
    const out = new Set<string>();
    for (const [id, values] of Object.entries(grid)) {
      values.forEach((v, c) => {
        if (v.trim() !== "" && !isAmount(v)) out.add(`${id}:${c}`);
      });
    }
    return out;
  }, [grid]);
  const dirty = changedCells.size > 0 || bandsChanged;

  const focusCell = useCallback((r: number, c: number) => {
    const el = inputs.current.get(`${r}:${c}`);
    if (el) {
      el.focus();
      el.select();
    }
  }, []);

  const setCell = (id: string, c: number, value: string) =>
    setGrid((prev) => {
      const row = [...(prev[id] ?? bands.map(() => ""))];
      row[c] = value;
      return { ...prev, [id]: row };
    });

  const onKeyDown = (
    e: KeyboardEvent<HTMLInputElement>,
    r: number,
    c: number,
  ) => {
    const el = e.currentTarget;
    const atStart = el.selectionStart === 0 && el.selectionEnd === 0;
    const atEnd = el.selectionStart === el.value.length;
    const allSelected =
      el.selectionStart === 0 && el.selectionEnd === el.value.length;
    switch (e.key) {
      case "ArrowDown":
      case "Enter":
        e.preventDefault();
        focusCell(e.shiftKey && e.key === "Enter" ? r - 1 : r + 1, c);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusCell(r - 1, c);
        break;
      case "ArrowLeft":
        if (atStart || allSelected) {
          e.preventDefault();
          focusCell(r, c - 1);
        }
        break;
      case "ArrowRight":
        if (atEnd || allSelected) {
          e.preventDefault();
          focusCell(r, c + 1);
        }
        break;
      case "Escape": {
        const id = rows[r]?.rental_product.uuid;
        if (!id || !matrix) break;
        const si = matrix.bands.findIndex(
          (b) => bandKey(b) === bandKey(bands[c]),
        );
        setCell(id, c, si >= 0 ? (baseline[id]?.[si] ?? "") : "");
        break;
      }
    }
  };

  // Paste a block copied from Excel / Sheets (tabs + newlines).
  const onPaste = (
    e: ClipboardEvent<HTMLInputElement>,
    r: number,
    c: number,
  ) => {
    const text = e.clipboardData.getData("text/plain");
    if (!text.includes("\t") && !text.includes("\n")) return;
    e.preventDefault();
    const lines = text
      .replace(/\r/g, "")
      .split("\n")
      .filter((l) => l !== "");
    setGrid((prev) => {
      const next = { ...prev };
      lines.forEach((line, dr) => {
        const row = rows[r + dr];
        if (!row) return;
        const id = row.rental_product.uuid;
        const values = [...(next[id] ?? bands.map(() => ""))];
        line.split("\t").forEach((cell, dc) => {
          if (c + dc < bands.length) values[c + dc] = cell.trim();
        });
        next[id] = values;
      });
      return next;
    });
  };

  const addBand = () => {
    const min = Number(newBand.min);
    const max = newBand.max.trim() === "" ? null : Number(newBand.max);
    if (
      !Number.isInteger(min) ||
      min < 1 ||
      (max != null && (!Number.isInteger(max) || max < min))
    ) {
      return;
    }
    const next = [...bands, { min_days: min, max_days: max }].sort(
      (a, b) => a.min_days - b.min_days,
    );
    const index = next.findIndex(
      (b) => b.min_days === min && b.max_days === max,
    );
    setBands(next);
    setGrid((prev) => {
      const out: Grid = {};
      for (const [id, values] of Object.entries(prev)) {
        const copy = [...values];
        copy.splice(index, 0, "");
        out[id] = copy;
      }
      return out;
    });
    setNewBand({ min: "", max: "" });
    setAddOpen(false);
  };

  const removeBand = (c: number) => {
    setBands((prev) => prev.filter((_, i) => i !== c));
    setGrid((prev) => {
      const out: Grid = {};
      for (const [id, values] of Object.entries(prev)) {
        out[id] = values.filter((_, i) => i !== c);
      }
      return out;
    });
  };

  const reset = () => {
    if (!matrix) return;
    setBands(matrix.bands.length ? matrix.bands : bands);
    setGrid(baseline);
  };

  const submit = () => {
    if (!matrix) return;
    save.mutate({
      scope,
      branch_uuid: query.branch ?? null,
      channel: query.channel ?? null,
      priority: query.priority ?? 0,
      bands,
      rows: rows.map((row) => ({
        rental_product_uuid: row.rental_product.uuid,
        prices: (grid[row.rental_product.uuid] ?? []).map((v) =>
          v.trim() === "" ? null : normalizeAmount(v),
        ),
      })),
    });
  };

  const overlapping = bands.some(
    (b, i) =>
      i > 0 &&
      (bands[i - 1].max_days == null ||
        (bands[i - 1].max_days ?? 0) >= b.min_days),
  );
  const columns = `minmax(14rem, 1.6fr) repeat(${Math.max(bands.length, 1)}, minmax(7.5rem, 1fr)) minmax(8rem, 0.8fr)`;
  const activateIssues = issuesFromError(activate.error);

  return (
    <EntityPage
      title={t("pricing.matrix.title")}
      description={t("pricing.matrix.description")}
      permission={permissions.tenant.pricing.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("pricing.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("pricing.plans.title"),
          href: routes.tenant.pricing.plans.root(slug),
        },
        { label: t("pricing.matrix.title") },
      ]}
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <ResourceIOToolbar
            resource={RATE_PLANS_RESOURCE}
            query={{ scope }}
            capabilities={metaQuery.data?.capabilities}
            onImportComplete={() => void matrixQuery.refetch()}
          />
          <EntityToolbar
            onRefresh={() => void matrixQuery.refetch()}
            refreshDisabled={matrixQuery.isFetching}
          />
        </div>
      }
    >
      <div className="space-y-6">
        <EntitySectionCard title={t("pricing.matrix.target")}>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="matrix-scope">{t("pricing.fields.scope")}</Label>
              <Select
                value={scope}
                onValueChange={(v) =>
                  setScope(v as Exclude<RatePlanScope, "agreement">)
                }
              >
                <SelectTrigger id="matrix-scope">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PLAN_SCOPES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(SCOPE_LABEL_KEYS[s])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-muted-foreground text-xs">
                {t(SCOPE_HINT_KEYS[scope])}
              </p>
            </div>
            {BRANCH_SCOPES.includes(scope) ? (
              <div className="space-y-2">
                <Label htmlFor="matrix-branch">
                  {t("pricing.fields.branch")}
                </Label>
                <Select value={branch} onValueChange={setBranch}>
                  <SelectTrigger id="matrix-branch">
                    <SelectValue
                      placeholder={t("pricing.fields.branch_placeholder")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {branches.options.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : null}
            {CHANNEL_SCOPES.includes(scope) ? (
              <div className="space-y-2">
                <Label htmlFor="matrix-channel">
                  {t("pricing.fields.channel")}
                </Label>
                <Select value={channel} onValueChange={setChannel}>
                  <SelectTrigger id="matrix-channel">
                    <SelectValue
                      placeholder={t("pricing.fields.channel_placeholder")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {channels.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : null}
            <div className="space-y-2">
              <Label htmlFor="matrix-priority">
                {t("pricing.fields.priority")}
              </Label>
              <Input
                id="matrix-priority"
                inputMode="numeric"
                value={priority}
                onChange={(e) =>
                  setPriority(e.target.value.replace(/\D/g, "").slice(0, 4))
                }
              />
            </div>
          </div>
        </EntitySectionCard>

        {!targetReady ? (
          <EmptyState
            className="py-10"
            title={t("pricing.matrix.pick_target")}
            description={t("pricing.matrix.pick_target_hint")}
          />
        ) : null}
        {targetReady && matrixQuery.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : null}
        {targetReady && matrixQuery.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("pricing.matrix.load_failed")}
            onRetry={() => void matrixQuery.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}

        {targetReady && matrix ? (
          <EntitySectionCard
            title={t("pricing.matrix.grid_title")}
            badge={rows.length}
          >
            <div className="space-y-4">
              <p className="text-muted-foreground text-sm">
                {canWrite
                  ? t("pricing.matrix.help_edit")
                  : t("pricing.matrix.help_view")}
              </p>
              {rows.length === 0 ? (
                <EmptyState
                  className="py-10"
                  title={t("pricing.matrix.no_products")}
                  description={t("pricing.matrix.no_products_hint")}
                />
              ) : (
                <div
                  className="overflow-x-auto rounded-lg border"
                  data-testid="rate-matrix"
                >
                  <div
                    role="grid"
                    aria-label={t("pricing.matrix.grid_title")}
                    aria-rowcount={rows.length + 1}
                    style={{ minWidth: `${22 + bands.length * 7.5}rem` }}
                  >
                    <div
                      role="row"
                      className="bg-muted/50 grid border-b text-xs font-medium"
                      style={{ gridTemplateColumns: columns }}
                    >
                      <div role="columnheader" className="px-3 py-2">
                        {t("pricing.columns.rental_product")}
                      </div>
                      {bands.map((b, c) => (
                        <div
                          key={bandKey(b)}
                          role="columnheader"
                          className="flex items-center justify-between gap-1 px-2 py-2"
                        >
                          <span className="tabular-nums">
                            {bandLabel(t, b.min_days, b.max_days)}
                          </span>
                          {canWrite ? (
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="size-6"
                              aria-label={t("pricing.matrix.remove_band")}
                              onClick={() => removeBand(c)}
                            >
                              <X className="size-3" />
                            </Button>
                          ) : null}
                        </div>
                      ))}
                      <div role="columnheader" className="px-3 py-2">
                        {t("pricing.columns.status")}
                      </div>
                    </div>
                    {rows.map((row, r) => {
                      const id = row.rental_product.uuid;
                      return (
                        <div
                          key={id}
                          role="row"
                          aria-rowindex={r + 2}
                          className="hover:bg-muted/30 grid border-b last:border-b-0"
                          style={{ gridTemplateColumns: columns }}
                        >
                          <div
                            role="rowheader"
                            className="flex flex-col justify-center px-3 py-1.5"
                          >
                            <span className="text-sm font-medium">
                              <code className="text-xs">
                                {row.rental_product.code}
                              </code>{" "}
                              {row.rental_product.title}
                            </span>
                            <span className="text-muted-foreground text-xs">
                              {row.segment.name}
                            </span>
                          </div>
                          {bands.map((b, c) => {
                            const key = `${id}:${c}`;
                            const changed = changedCells.has(key);
                            const invalid = invalidCells.has(key);
                            return (
                              <div
                                key={bandKey(b)}
                                role="gridcell"
                                className="px-1 py-1"
                              >
                                <Input
                                  ref={(el) => {
                                    if (el) inputs.current.set(`${r}:${c}`, el);
                                    else inputs.current.delete(`${r}:${c}`);
                                  }}
                                  value={grid[id]?.[c] ?? ""}
                                  readOnly={!canWrite}
                                  inputMode="decimal"
                                  placeholder="—"
                                  aria-label={`${row.rental_product.code} ${bandLabel(t, b.min_days, b.max_days)}`}
                                  aria-invalid={invalid || undefined}
                                  className={cn(
                                    "h-8 text-right tabular-nums",
                                    changed &&
                                      "border-amber-500 bg-amber-50 dark:bg-amber-950/30",
                                    invalid && "border-destructive",
                                  )}
                                  onFocus={(e) => e.currentTarget.select()}
                                  onChange={(e) =>
                                    setCell(id, c, e.target.value)
                                  }
                                  onKeyDown={(e) => onKeyDown(e, r, c)}
                                  onPaste={(e) => onPaste(e, r, c)}
                                />
                              </div>
                            );
                          })}
                          <div
                            role="gridcell"
                            className="flex flex-wrap items-center gap-1 px-3 py-1.5"
                          >
                            {row.plan_status ? (
                              <>
                                {row.plan_uuid ? (
                                  <Link
                                    className="underline-offset-2 hover:underline"
                                    href={routes.tenant.pricing.plans.detail(
                                      slug,
                                      row.plan_uuid,
                                    )}
                                  >
                                    <StatusChip
                                      label={t(
                                        STATUS_LABEL_KEYS[row.plan_status],
                                      )}
                                      tone={STATUS_TONES[row.plan_status]}
                                    />
                                  </Link>
                                ) : null}
                                {row.has_draft &&
                                row.plan_status !== "draft" ? (
                                  <StatusChip
                                    label={t("pricing.pending_draft")}
                                    tone="warning"
                                  />
                                ) : null}
                              </>
                            ) : (
                              <Badge variant="outline">
                                {t("pricing.matrix.no_plan")}
                              </Badge>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}

              {overlapping ? (
                <p className="text-destructive text-sm">
                  {t("pricing.validation.overlap")}
                </p>
              ) : null}

              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="text-muted-foreground flex flex-wrap items-center gap-3 text-sm">
                  <span>
                    {t("pricing.matrix.changes", { count: changedCells.size })}
                  </span>
                  {matrix.drafts > 0 ? (
                    <StatusChip
                      label={t("pricing.matrix.drafts", {
                        count: matrix.drafts,
                      })}
                      tone="warning"
                    />
                  ) : null}
                </div>
                <div className="flex flex-wrap gap-2">
                  {canWrite ? (
                    <>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setAddOpen(true)}
                      >
                        <Plus className="size-4" />
                        {t("pricing.matrix.add_band")}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={!dirty}
                        onClick={reset}
                      >
                        <RotateCcw className="size-4" />
                        {t("pricing.matrix.reset")}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        disabled={
                          !dirty ||
                          invalidCells.size > 0 ||
                          overlapping ||
                          save.isPending
                        }
                        onClick={submit}
                      >
                        <Save className="size-4" />
                        {t("pricing.actions.save_draft")}
                      </Button>
                    </>
                  ) : null}
                  <PermissionGuard
                    permission={permissions.tenant.pricing.activate}
                  >
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={matrix.drafts === 0 || dirty}
                      onClick={() => {
                        activate.reset();
                        setActivateOpen(true);
                      }}
                    >
                      <CheckCircle2 className="size-4" />
                      {t("pricing.matrix.activate_drafts")}
                    </Button>
                  </PermissionGuard>
                </div>
              </div>
              {activateIssues.length > 0 ? (
                <IssueList issues={activateIssues} />
              ) : null}
            </div>
          </EntitySectionCard>
        ) : null}
      </div>

      <AppDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        title={t("pricing.matrix.add_band")}
        description={t("pricing.matrix.add_band_hint")}
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setAddOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              onClick={addBand}
              disabled={!/^\d{1,4}$/.test(newBand.min)}
            >
              {t("pricing.matrix.add_band")}
            </Button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-2">
            <Label htmlFor="band-min">{t("pricing.fields.min_days")}</Label>
            <Input
              id="band-min"
              inputMode="numeric"
              value={newBand.min}
              onChange={(e) =>
                setNewBand((p) => ({
                  ...p,
                  min: e.target.value.replace(/\D/g, ""),
                }))
              }
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="band-max">{t("pricing.fields.max_days")}</Label>
            <Input
              id="band-max"
              inputMode="numeric"
              placeholder="∞"
              value={newBand.max}
              onChange={(e) =>
                setNewBand((p) => ({
                  ...p,
                  max: e.target.value.replace(/\D/g, ""),
                }))
              }
            />
          </div>
        </div>
      </AppDialog>

      <ConfirmDialog
        open={activateOpen}
        title={t("pricing.matrix.activate_title")}
        description={t("pricing.matrix.activate_body", {
          count: matrix?.drafts ?? 0,
        })}
        confirmLabel={t("pricing.actions.activate")}
        isPending={activate.isPending}
        onCancel={() => setActivateOpen(false)}
        onConfirm={() =>
          activate.mutate(
            {
              scope,
              branch_uuid: query.branch ?? null,
              channel: query.channel ?? null,
              priority: query.priority ?? 0,
            },
            { onSettled: () => setActivateOpen(false) },
          )
        }
      />
    </EntityPage>
  );
}
