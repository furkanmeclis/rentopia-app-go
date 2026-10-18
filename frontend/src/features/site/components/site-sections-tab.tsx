"use client";

import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { restrictToVerticalAxis } from "@dnd-kit/modifiers";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { ArrowDown, ArrowUp, GripVertical, Inbox, Lock } from "lucide-react";
import Link from "next/link";
import {
  Controller,
  get,
  useFieldArray,
  useFormContext,
  useWatch,
} from "react-hook-form";

import { EmptyState } from "@/components/common/empty-state";
import { AppSwitch, FormSection } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  LEAD_FORMS,
  NEEDS_SERVICE,
  type LeadForm,
  SECTION_IDS,
  keySegment,
  type SectionId,
  type SectionStatus,
  type ThemeDefinition,
} from "@/features/site/lib/site-options";
import type { SiteFormValues } from "@/features/site/schemas/site-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type SectionLock =
  | { kind: "required" }
  | { kind: "needs_service"; ticket: string }
  | { kind: "app_promo" }
  | { kind: "unavailable" }
  | null;

/**
 * Why a section cannot be switched on. The API's `section_statuses` (for
 * the saved template) wins; other templates use the same service rules.
 */
function lockOf(
  id: string,
  index: number,
  def: ThemeDefinition | undefined,
  statuses: SectionStatus[] | undefined,
): SectionLock {
  if (index === 0 && id === (def?.required_first ?? "hero")) {
    return { kind: "required" };
  }
  if (id === "app_promo") return { kind: "app_promo" };
  const ticket = NEEDS_SERVICE[id as SectionId];
  const status = statuses?.find((s) => s.id === id);
  if (status?.status === "needs_service" || ticket) {
    return { kind: "needs_service", ticket: ticket ?? "" };
  }
  if (
    status &&
    status.status !== "unsupported_by_theme" &&
    !status.can_enable
  ) {
    return { kind: "unavailable" };
  }
  return null;
}

function SectionRow({
  id,
  index,
  count,
  lock,
  disabled,
  onMove,
}: {
  id: string;
  index: number;
  count: number;
  lock: SectionLock;
  disabled?: boolean;
  onMove: (from: number, to: number) => void;
}) {
  const { t } = useLocale();
  const pinned = lock?.kind === "required";
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled: disabled || pinned });
  const k = keySegment(id);
  const switchLocked = lock !== null;
  const newsletterOn = useWatch<SiteFormValues, "config.leads.newsletter">({
    name: "config.leads.newsletter",
  });
  return (
    <li
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={cn(
        "bg-card flex items-start gap-3 rounded-lg border p-3",
        isDragging && "ring-primary/40 relative z-10 shadow-md ring-2",
      )}
    >
      <button
        type="button"
        className={cn(
          "text-muted-foreground/70 hover:text-foreground mt-0.5 inline-flex size-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-md",
          "focus-visible:ring-ring outline-none focus-visible:ring-2 active:cursor-grabbing",
          "disabled:cursor-not-allowed disabled:opacity-40",
        )}
        aria-label={t("site.sections.drag", {
          name: t(`site.sections.names.${k}`),
        })}
        disabled={disabled || pinned}
        {...attributes}
        {...listeners}
      >
        {pinned ? (
          <Lock className="size-4" aria-hidden />
        ) : (
          <GripVertical className="size-4" aria-hidden />
        )}
      </button>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">
            {t(`site.sections.names.${k}`)}
          </span>
          {lock?.kind === "needs_service" ? (
            <Badge variant="secondary">
              {t("site.sections.badge_service")}
            </Badge>
          ) : lock?.kind === "app_promo" ? (
            <Badge variant="secondary">{t("site.sections.badge_soon")}</Badge>
          ) : pinned ? (
            <Badge variant="outline">{t("site.sections.badge_required")}</Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground text-xs">
          {t(`site.sections.descriptions.${k}`)}
        </p>
        {lock?.kind === "needs_service" ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.needs_service", {
              feature: t(`site.sections.services.${k}`),
              ticket: lock.ticket,
            })}
          </p>
        ) : lock?.kind === "app_promo" ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.app_promo_locked")}
          </p>
        ) : lock?.kind === "required" ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.required_first")}
          </p>
        ) : lock?.kind === "unavailable" ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.unavailable")}
          </p>
        ) : id === "newsletter" && newsletterOn === false ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.newsletter_form_off")}
          </p>
        ) : id === "campaigns" ? (
          <p className="text-muted-foreground text-xs">
            {t("site.sections.campaigns_hint")}
          </p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {!disabled && !pinned ? (
          <>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              disabled={index <= 1}
              aria-label={t("site.sections.up")}
              onClick={() => onMove(index, index - 1)}
            >
              <ArrowUp className="size-4" />
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              disabled={index >= count - 1}
              aria-label={t("site.sections.down")}
              onClick={() => onMove(index, index + 1)}
            >
              <ArrowDown className="size-4" />
            </Button>
          </>
        ) : null}
      </div>
      <SectionSwitch
        index={index}
        name={t(`site.sections.names.${k}`)}
        locked={switchLocked}
        forceOn={pinned}
        disabled={disabled}
      />
    </li>
  );
}

/** The enabled switch, bound through the field array path of the row. */
function SectionSwitch({
  index,
  name,
  locked,
  forceOn,
  disabled,
}: {
  index: number;
  name: string;
  locked: boolean;
  forceOn: boolean;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const { control } = useFormContext<SiteFormValues>();
  const template = useWatch({ control, name: "config.template" });
  return (
    <Controller
      name={`config.templates.${template}.sections.${index}.enabled`}
      control={control}
      render={({ field }) => (
        <Switch
          className="mt-1 shrink-0"
          checked={forceOn || Boolean(field.value)}
          onCheckedChange={field.onChange}
          disabled={disabled || (locked && !field.value) || forceOn}
          aria-label={t("site.sections.toggle", { name })}
        />
      )}
    />
  );
}

/**
 * Bölümler: the active template's sections in their order (drag or the
 * arrows), each with a visibility switch. Hero stays first and on; sections
 * waiting for a backend service or for store links cannot be switched on;
 * sections the theme lacks are listed separately. Every template keeps its
 * own list (`config.templates.{id}`).
 */
export function SiteSectionsTab({
  registry,
  statuses,
  savedTemplate,
  disabled,
}: {
  registry: ThemeDefinition[];
  statuses: SectionStatus[] | undefined;
  /** The template the API's statuses describe (the saved draft's). */
  savedTemplate: string;
  disabled?: boolean;
}) {
  const template = useWatch<SiteFormValues, "config.template">({
    name: "config.template",
  });
  return (
    <div className="space-y-4">
      <SectionList
        key={template}
        template={template}
        def={registry.find((d) => d.id === template)}
        statuses={template === savedTemplate ? statuses : undefined}
        disabled={disabled}
      />
      <SiteFormsSection disabled={disabled} />
    </div>
  );
}

/**
 * Formlar: which website forms are offered (`config.leads`, TEC-786).
 * Draft content like the rest of the document: it applies once published.
 * Submissions land in Ayarlar → Site başvuruları.
 */
const FORM_KEYS: Record<LeadForm, { label: string; hint: string }> = {
  contact: { label: "site.forms.contact", hint: "site.forms.contact_hint" },
  corporate: {
    label: "site.forms.corporate",
    hint: "site.forms.corporate_hint",
  },
  franchise: {
    label: "site.forms.franchise",
    hint: "site.forms.franchise_hint",
  },
  newsletter: {
    label: "site.forms.newsletter",
    hint: "site.forms.newsletter_hint",
  },
};

function SiteFormsSection({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  return (
    <FormSection
      title={t("site.forms.title")}
      description={t("site.forms.description")}
      columns={2}
    >
      {LEAD_FORMS.map((form) => (
        <AppSwitch
          key={form}
          name={`config.leads.${form}`}
          label={t(FORM_KEYS[form].label)}
          description={t(FORM_KEYS[form].hint)}
          disabled={disabled}
        />
      ))}
      <p className="text-muted-foreground text-xs sm:col-span-2">
        {t("site.forms.note")}
      </p>
      {can(permissions.tenant.siteLeads.read) ? (
        <div className="sm:col-span-2">
          <Button asChild type="button" size="sm" variant="outline">
            <Link href={routes.tenant.settings.siteLeads(slug)}>
              <Inbox className="size-4" />
              {t("site.forms.leads_link")}
            </Link>
          </Button>
        </div>
      ) : null}
    </FormSection>
  );
}

function SectionList({
  template,
  def,
  statuses,
  disabled,
}: {
  template: string;
  def: ThemeDefinition | undefined;
  statuses: SectionStatus[] | undefined;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<SiteFormValues>();
  const { fields, move } = useFieldArray({
    control,
    name: `config.templates.${template}.sections`,
    keyName: "key",
  });
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const ids = fields.map((f) => f.id);
  const safeMove = (from: number, to: number) => {
    // The required first section (hero) never moves and nothing goes above it.
    if (from <= 0 || to <= 0 || to >= fields.length) return;
    move(from, to);
  };
  const onDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    safeMove(ids.indexOf(String(active.id)), ids.indexOf(String(over.id)));
  };
  const missing = SECTION_IDS.filter((id) => def && !def.sections.includes(id));
  const themeName = t(`site.templates.${keySegment(template)}.name`);
  const listErrors = get(errors, `config.templates.${template}.sections`);
  const rowErrors: string[] = [];
  if (listErrors?.message) rowErrors.push(String(listErrors.message));
  if (listErrors?.root?.message)
    rowErrors.push(String(listErrors.root.message));
  fields.forEach((field, index) => {
    const message =
      listErrors?.[index]?.message ?? listErrors?.[index]?.enabled?.message;
    if (message) {
      rowErrors.push(
        `${t(`site.sections.names.${keySegment(field.id)}`)}: ${String(message)}`,
      );
    }
  });

  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.sections.title")}
        description={t("site.sections.description", { theme: themeName })}
      >
        {rowErrors.length ? (
          <ul className="text-destructive space-y-1 text-sm">
            {rowErrors.map((message) => (
              <li key={message}>{message}</li>
            ))}
          </ul>
        ) : null}
        {fields.length === 0 ? (
          <EmptyState
            title={t("site.sections.empty_title")}
            description={t("site.sections.empty_description")}
          />
        ) : (
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={onDragEnd}
          >
            <SortableContext items={ids} strategy={verticalListSortingStrategy}>
              <ol className="space-y-2">
                {fields.map((field, index) => (
                  <SectionRow
                    key={field.key}
                    id={field.id}
                    index={index}
                    count={fields.length}
                    lock={lockOf(field.id, index, def, statuses)}
                    disabled={disabled}
                    onMove={safeMove}
                  />
                ))}
              </ol>
            </SortableContext>
          </DndContext>
        )}
      </FormSection>
      {missing.length ? (
        <FormSection
          title={t("site.sections.missing_title")}
          description={t("site.sections.missing_description", {
            theme: themeName,
          })}
        >
          <ul className="flex flex-wrap gap-2">
            {missing.map((id) => (
              <li key={id}>
                <Badge variant="outline" className="text-muted-foreground">
                  {t(`site.sections.names.${keySegment(id)}`)}
                </Badge>
              </li>
            ))}
          </ul>
        </FormSection>
      ) : null}
    </div>
  );
}
