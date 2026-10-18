"use client";

import { Plus, Wand2, X } from "lucide-react";
import { useState } from "react";
import { Controller, get, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group";
import {
  ARTICLE_FIELD_LIMITS,
  slugify,
  type ArticleFormValues,
} from "@/features/site-articles/lib/article-form";
import type { ArticleLocale } from "@/features/site-articles/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Slug with the public path as prefix. While `auto` is on it follows the
 * title (the parent writes it); typing turns it off, the wand turns it on.
 */
export function SlugField({
  locale,
  auto,
  onAutoChange,
  disabled,
}: {
  locale: ArticleLocale;
  auto: boolean;
  onAutoChange: (auto: boolean) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const name = `slug.${locale}` as const;
  const {
    control,
    getValues,
    setValue,
    formState: { errors },
  } = useFormContext<ArticleFormValues>();
  const error = get(errors, name)?.message as string | undefined;
  return (
    <FormFieldShell
      name={name}
      label={t("site.articles.fields.slug")}
      description={t(
        auto
          ? "site.articles.fields.slug_auto_hint"
          : "site.articles.fields.slug_hint",
      )}
      error={error}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <InputGroup className={cn(error && "border-destructive")}>
            <InputGroupAddon align="inline-start">
              <InputGroupText className="font-mono text-xs">
                /guide/{locale}/
              </InputGroupText>
            </InputGroupAddon>
            <InputGroupInput
              id={name}
              value={field.value}
              onChange={(e) => {
                onAutoChange(false);
                field.onChange(e.target.value.toLowerCase());
              }}
              onBlur={field.onBlur}
              maxLength={ARTICLE_FIELD_LIMITS.slug}
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              disabled={disabled}
              aria-invalid={Boolean(error)}
            />
            {!disabled ? (
              <InputGroupAddon align="inline-end">
                <InputGroupButton
                  type="button"
                  size="icon-xs"
                  aria-label={t("site.articles.fields.slug_from_title")}
                  title={t("site.articles.fields.slug_from_title")}
                  onClick={() => {
                    onAutoChange(true);
                    setValue(name, slugify(getValues(`title.${locale}`)), {
                      shouldDirty: true,
                      shouldValidate: true,
                    });
                  }}
                >
                  <Wand2 />
                </InputGroupButton>
              </InputGroupAddon>
            ) : null}
          </InputGroup>
        )}
      />
    </FormFieldShell>
  );
}

/** Up to ten plain-text tags: Enter or comma adds, the chip's × removes. */
export function TagsField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<ArticleFormValues>();
  const [draft, setDraft] = useState("");
  const error =
    (get(errors, "tags")?.message as string | undefined) ??
    (Array.isArray(get(errors, "tags"))
      ? (get(errors, "tags") as { message?: string }[]).find((e) => e?.message)
          ?.message
      : undefined);
  return (
    <FormFieldShell
      name="tags-input"
      label={t("site.articles.fields.tags")}
      description={t("site.articles.fields.tags_hint", {
        max: ARTICLE_FIELD_LIMITS.tags,
      })}
      error={error}
    >
      <Controller
        name="tags"
        control={control}
        render={({ field }) => {
          const tags = (field.value ?? []) as string[];
          const full = tags.length >= ARTICLE_FIELD_LIMITS.tags;
          const add = () => {
            const value = draft
              .replace(/[<>]/g, "")
              .trim()
              .slice(0, ARTICLE_FIELD_LIMITS.tag);
            if (!value || full) return;
            if (!tags.some((x) => x.toLowerCase() === value.toLowerCase())) {
              field.onChange([...tags, value]);
            }
            setDraft("");
          };
          return (
            <div className="space-y-2">
              {tags.length ? (
                <ul className="flex flex-wrap gap-1.5">
                  {tags.map((tag) => (
                    <li key={tag}>
                      <Badge variant="secondary" className="gap-1 pe-1">
                        {tag}
                        {!disabled ? (
                          <Button
                            type="button"
                            size="icon-xs"
                            variant="ghost"
                            className="size-4"
                            aria-label={t("site.articles.fields.tag_remove", {
                              tag,
                            })}
                            onClick={() =>
                              field.onChange(tags.filter((x) => x !== tag))
                            }
                          >
                            <X className="size-3" />
                          </Button>
                        ) : null}
                      </Badge>
                    </li>
                  ))}
                </ul>
              ) : null}
              {!disabled ? (
                <div className="flex gap-2">
                  <Input
                    id="tags-input"
                    value={draft}
                    disabled={full}
                    maxLength={ARTICLE_FIELD_LIMITS.tag}
                    placeholder={t("site.articles.fields.tag_placeholder")}
                    onChange={(e) => setDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === ",") {
                        e.preventDefault();
                        add();
                      }
                    }}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    disabled={full || !draft.trim()}
                    aria-label={t("site.articles.fields.tag_add")}
                    onClick={add}
                  >
                    <Plus className="size-4" />
                  </Button>
                </div>
              ) : null}
            </div>
          );
        }}
      />
    </FormFieldShell>
  );
}

/** Integer field (the list's `sort`). */
export function SortField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<ArticleFormValues>();
  const error = get(errors, "sort")?.message as string | undefined;
  return (
    <FormFieldShell
      name="sort"
      label={t("site.articles.fields.sort")}
      description={t("site.articles.fields.sort_hint")}
      error={error}
    >
      <Controller
        name="sort"
        control={control}
        render={({ field }) => (
          <Input
            id="sort"
            type="number"
            inputMode="numeric"
            step={1}
            value={Number.isFinite(field.value) ? field.value : 0}
            onChange={(e) => {
              const n = Number.parseInt(e.target.value, 10);
              field.onChange(Number.isFinite(n) ? n : 0);
            }}
            onBlur={field.onBlur}
            disabled={disabled}
            aria-invalid={Boolean(error)}
          />
        )}
      />
    </FormFieldShell>
  );
}
