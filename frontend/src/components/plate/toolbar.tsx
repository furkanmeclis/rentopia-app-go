"use client";

import { toggleCodeBlock } from "@platejs/code-block";
import { unwrapLink, upsertLink } from "@platejs/link";
import {
  useListToolbarButton,
  useListToolbarButtonState,
} from "@platejs/list-classic/react";
import {
  Bold,
  ChevronDown,
  Code,
  FileCode,
  ImagePlus,
  Italic,
  Link2,
  Link2Off,
  List,
  ListOrdered,
  Minus,
  Quote,
  Redo2,
  Strikethrough,
  Underline,
  Undo2,
  type LucideIcon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import type { TRange } from "platejs";
import {
  useEditorReadOnly,
  useEditorRef,
  useEditorSelector,
  useMarkToolbarButton,
  useMarkToolbarButtonState,
} from "platejs/react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { Toggle } from "@/components/ui/toggle";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { normalizeHrefInput, validArticleHref } from "./value";
import { useLocale } from "@/providers/locale-provider";

function Tip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function MarkButton({
  mark,
  icon: Icon,
  label,
}: {
  mark: string;
  icon: LucideIcon;
  label: string;
}) {
  const state = useMarkToolbarButtonState({ nodeType: mark });
  const { props } = useMarkToolbarButton(state);
  return (
    <Tip label={label}>
      <Toggle
        size="sm"
        aria-label={label}
        pressed={props.pressed}
        onMouseDown={props.onMouseDown}
        onClick={props.onClick}
      >
        <Icon />
      </Toggle>
    </Tip>
  );
}

function ListButton({
  type,
  icon: Icon,
  label,
}: {
  type: "ul" | "ol";
  icon: LucideIcon;
  label: string;
}) {
  const state = useListToolbarButtonState({ nodeType: type });
  const { props } = useListToolbarButton(state);
  return (
    <Tip label={label}>
      <Toggle
        size="sm"
        aria-label={label}
        pressed={props.pressed}
        onMouseDown={props.onMouseDown}
        onClick={props.onClick}
      >
        <Icon />
      </Toggle>
    </Tip>
  );
}

function BlockToggle({
  type,
  icon: Icon,
  label,
  onToggle,
}: {
  type: string;
  icon: LucideIcon;
  label: string;
  onToggle?: () => void;
}) {
  const editor = useEditorRef();
  const pressed = useEditorSelector(
    (e) => Boolean(e.selection) && e.api.some({ match: { type } }),
    [type],
  );
  return (
    <Tip label={label}>
      <Toggle
        size="sm"
        aria-label={label}
        pressed={pressed}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (onToggle) onToggle();
          else editor.tf.toggleBlock(type);
          editor.tf.focus();
        }}
      >
        <Icon />
      </Toggle>
    </Tip>
  );
}

const HEADINGS = ["p", "h2", "h3", "h4"] as const;

function BlockTypeMenu() {
  const { t } = useLocale();
  const editor = useEditorRef();
  const current = useEditorSelector((e) => {
    const entry = e.api.block();
    const type = entry ? String((entry[0] as { type?: unknown }).type) : "p";
    return (HEADINGS as readonly string[]).includes(type) ? type : "p";
  }, []);
  const label = (type: string) => t(`editor.rich_text.block.${type}`);
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-w-28 justify-between"
          onMouseDown={(e) => e.preventDefault()}
          aria-label={t("editor.rich_text.block_type")}
        >
          {label(current)}
          <ChevronDown className="size-3.5 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          editor.tf.focus();
        }}
      >
        <DropdownMenuRadioGroup
          value={current}
          onValueChange={(type) => {
            editor.tf.toggleBlock(type);
          }}
        >
          {HEADINGS.map((type) => (
            <DropdownMenuRadioItem key={type} value={type}>
              {label(type)}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Link editor: relative path, https:// or mailto: only (the API's rule). */
function LinkButton() {
  const { t } = useLocale();
  const editor = useEditorRef();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [error, setError] = useState(false);
  const [saved, setSaved] = useState<TRange | null>(null);
  const active = useEditorSelector(
    (e) => Boolean(e.selection) && e.api.some({ match: { type: "a" } }),
    [],
  );

  const openEditor = (next: boolean) => {
    if (next) {
      const entry = editor.api.above({ match: { type: "a" } });
      const url = entry
        ? String((entry[0] as { url?: unknown }).url ?? "")
        : "";
      setSaved(editor.selection);
      setValue(url);
      setError(false);
    }
    setOpen(next);
  };

  const apply = () => {
    const href = normalizeHrefInput(value);
    if (!validArticleHref(href)) {
      setError(true);
      return;
    }
    if (saved) editor.tf.select(saved);
    upsertLink(editor, { url: href, skipValidation: true });
    setOpen(false);
    editor.tf.focus();
  };

  return (
    <>
      <Popover open={open} onOpenChange={openEditor}>
        <Tip label={t("editor.link")}>
          <PopoverTrigger asChild>
            <Toggle
              size="sm"
              aria-label={t("editor.link")}
              pressed={active}
              onMouseDown={(e) => e.preventDefault()}
            >
              <Link2 />
            </Toggle>
          </PopoverTrigger>
        </Tip>
        <PopoverContent align="start" className="w-80 space-y-2">
          <Label htmlFor="rich-text-link">
            {t("editor.rich_text.link_label")}
          </Label>
          <Input
            id="rich-text-link"
            value={value}
            autoFocus
            placeholder="https://… /vehicles mailto:…"
            aria-invalid={error}
            onChange={(e) => {
              setValue(e.target.value);
              setError(false);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                apply();
              }
            }}
          />
          <p
            className={
              error
                ? "text-destructive text-xs"
                : "text-muted-foreground text-xs"
            }
          >
            {t(
              error
                ? "editor.rich_text.link_invalid"
                : "editor.rich_text.link_hint",
            )}
          </p>
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="button" size="sm" onClick={apply}>
              {t("editor.rich_text.link_apply")}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      {active ? (
        <Tip label={t("editor.rich_text.link_remove")}>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t("editor.rich_text.link_remove")}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              unwrapLink(editor);
              editor.tf.focus();
            }}
          >
            <Link2Off />
          </Button>
        </Tip>
      ) : null}
    </>
  );
}

/**
 * The fixed toolbar: exactly the node types and marks the API accepts
 * (no colors, alignment, tables or embeds).
 */
export function RichTextToolbar({
  onInsertImage,
}: {
  onInsertImage?: () => void;
}) {
  const { t } = useLocale();
  const editor = useEditorRef();
  const readOnly = useEditorReadOnly();
  if (readOnly) return null;
  const sep = <Separator orientation="vertical" className="mx-1 h-6" />;
  return (
    <div
      role="toolbar"
      aria-label={t("editor.rich_text.toolbar")}
      className="bg-muted/30 flex flex-wrap items-center gap-0.5 border-b p-1"
    >
      <Tip label={t("editor.undo")}>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t("editor.undo")}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => editor.undo()}
        >
          <Undo2 />
        </Button>
      </Tip>
      <Tip label={t("editor.redo")}>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t("editor.redo")}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => editor.redo()}
        >
          <Redo2 />
        </Button>
      </Tip>
      {sep}
      <BlockTypeMenu />
      {sep}
      <MarkButton mark="bold" icon={Bold} label={t("editor.bold")} />
      <MarkButton mark="italic" icon={Italic} label={t("editor.italic")} />
      <MarkButton
        mark="underline"
        icon={Underline}
        label={t("editor.underline")}
      />
      <MarkButton
        mark="strikethrough"
        icon={Strikethrough}
        label={t("editor.strikethrough")}
      />
      <MarkButton mark="code" icon={Code} label={t("editor.code")} />
      {sep}
      <ListButton
        type="ul"
        icon={List}
        label={t("editor.rich_text.bulleted_list")}
      />
      <ListButton
        type="ol"
        icon={ListOrdered}
        label={t("editor.rich_text.numbered_list")}
      />
      <BlockToggle
        type="blockquote"
        icon={Quote}
        label={t("editor.rich_text.quote")}
      />
      <BlockToggle
        type="code_block"
        icon={FileCode}
        label={t("editor.rich_text.code_block")}
        onToggle={() => toggleCodeBlock(editor)}
      />
      {sep}
      <LinkButton />
      {onInsertImage ? (
        <Tip label={t("editor.rich_text.image")}>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t("editor.rich_text.image")}
            onMouseDown={(e) => e.preventDefault()}
            onClick={onInsertImage}
          >
            <ImagePlus />
          </Button>
        </Tip>
      ) : null}
      <Tip label={t("editor.rich_text.divider")}>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t("editor.rich_text.divider")}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            editor.tf.insertNodes({ type: "hr", children: [{ text: "" }] });
            editor.tf.focus();
          }}
        >
          <Minus />
        </Button>
      </Tip>
    </div>
  );
}
