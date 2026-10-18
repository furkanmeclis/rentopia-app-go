"use client";

import {
  BlockquotePlugin,
  BoldPlugin,
  CodePlugin,
  H2Plugin,
  H3Plugin,
  H4Plugin,
  HorizontalRulePlugin,
  ItalicPlugin,
  StrikethroughPlugin,
  UnderlinePlugin,
} from "@platejs/basic-nodes/react";
import { CodeBlockPlugin, CodeLinePlugin } from "@platejs/code-block/react";
import { LinkPlugin } from "@platejs/link/react";
import {
  BulletedListPlugin,
  ListItemContentPlugin,
  ListItemPlugin,
  ListPlugin,
  NumberedListPlugin,
} from "@platejs/list-classic/react";
import { TrailingBlockPlugin, type TRange, type Value } from "platejs";
import {
  ParagraphPlugin,
  Plate,
  PlateContent,
  createPlatePlugin,
  usePlateEditor,
} from "platejs/react";
import { useMemo, useState, type ReactNode } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  BlockquoteElement,
  BulletedListElement,
  CodeBlockElement,
  CodeLeaf,
  CodeLineElement,
  H2Element,
  H3Element,
  H4Element,
  HrElement,
  ImageElement,
  LinkElement,
  ListItemContentElement,
  ListItemElement,
  NumberedListElement,
  ParagraphElement,
  RichTextMediaContext,
  type ImageNode,
  type RichTextMedia,
} from "./elements";
import { RichTextToolbar } from "./toolbar";
import {
  ARTICLE_LIMITS,
  bodyStats,
  overLimits,
  plainText,
  serializeBody,
  validArticleHref,
  type PlateNode,
} from "./value";

/** Library image (void block): `media_uuid` + plain `alt`, no stored URL. */
const ImagePlugin = createPlatePlugin({
  key: "img",
  node: { isElement: true, isVoid: true, component: ImageElement },
});

/**
 * Exactly the API's allowlist (docs/design/branded-site.md §13). Pasted
 * content of other kinds falls back to paragraphs; whatever survives is
 * filtered again by `serializeBody` before saving.
 */
const PLUGINS = [
  ParagraphPlugin.withComponent(ParagraphElement),
  H2Plugin.configure({
    node: { component: H2Element },
    shortcuts: { toggle: { keys: "mod+alt+2" } },
  }),
  H3Plugin.configure({
    node: { component: H3Element },
    shortcuts: { toggle: { keys: "mod+alt+3" } },
  }),
  H4Plugin.configure({
    node: { component: H4Element },
    shortcuts: { toggle: { keys: "mod+alt+4" } },
  }),
  BlockquotePlugin.configure({
    node: { component: BlockquoteElement },
    shortcuts: { toggle: { keys: "mod+shift+period" } },
  }),
  HorizontalRulePlugin.withComponent(HrElement),
  CodeBlockPlugin.configure({
    node: { component: CodeBlockElement },
    options: { lowlight: null },
  }),
  CodeLinePlugin.withComponent(CodeLineElement),
  ListPlugin,
  BulletedListPlugin.withComponent(BulletedListElement),
  NumberedListPlugin.withComponent(NumberedListElement),
  ListItemPlugin.withComponent(ListItemElement),
  ListItemContentPlugin.withComponent(ListItemContentElement),
  LinkPlugin.configure({
    node: { component: LinkElement },
    options: {
      allowedSchemes: ["https", "mailto"],
      isUrl: (text: string) =>
        /^(https:\/\/|mailto:)/i.test(text) && validArticleHref(text),
    },
  }),
  ImagePlugin,
  BoldPlugin.configure({ shortcuts: { toggle: { keys: "mod+b" } } }),
  ItalicPlugin.configure({ shortcuts: { toggle: { keys: "mod+i" } } }),
  UnderlinePlugin.configure({ shortcuts: { toggle: { keys: "mod+u" } } }),
  StrikethroughPlugin.configure({
    shortcuts: { toggle: { keys: "mod+shift+x" } },
  }),
  CodePlugin.configure({
    node: { component: CodeLeaf },
    shortcuts: { toggle: { keys: "mod+e" } },
  }),
  TrailingBlockPlugin,
];

export type ImagePickerRender = (props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (uuid: string) => void;
}) => ReactNode;

type AltState =
  | { mode: "insert"; uuid: string; alt: string; at: TRange | null }
  | { mode: "edit"; path: number[]; alt: string }
  | null;

/**
 * Article body editor (Plate, TEC-785). Uncontrolled: `initialValue` is
 * read once (remount with a new `key` to load another document);
 * `onChange` receives the Plate value, which `serializeBody` turns into
 * the API body.
 */
export function RichTextEditor({
  id,
  initialValue,
  onChange,
  disabled,
  placeholder,
  resolveMedia,
  renderImagePicker,
  invalid,
  lang,
}: {
  id?: string;
  initialValue: PlateNode[];
  onChange: (value: PlateNode[]) => void;
  disabled?: boolean;
  placeholder?: string;
  resolveMedia: RichTextMedia["resolve"];
  renderImagePicker?: ImagePickerRender;
  invalid?: boolean;
  /** Language of the text (spell checking). */
  lang?: string;
}) {
  const { t } = useLocale();
  const editor = usePlateEditor({
    plugins: PLUGINS,
    value: initialValue as Value,
    nodeId: false,
  });
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickAt, setPickAt] = useState<TRange | null>(null);
  const [altState, setAltState] = useState<AltState>(null);
  const [stats, setStats] = useState(() =>
    bodyStats(serializeBody(initialValue).nodes),
  );

  const media = useMemo<RichTextMedia>(
    () => ({
      resolve: resolveMedia,
      editAlt: (element: ImageNode, path: number[]) =>
        setAltState({ mode: "edit", path, alt: String(element.alt ?? "") }),
    }),
    [resolveMedia],
  );

  const commitAlt = () => {
    if (!altState) return;
    const alt = plainText(altState.alt.trim()).slice(0, ARTICLE_LIMITS.alt);
    if (altState.mode === "insert") {
      if (altState.at) editor.tf.select(altState.at);
      editor.tf.insertNodes({
        type: "img",
        media_uuid: altState.uuid,
        alt,
        children: [{ text: "" }],
      } as ImageNode);
    } else {
      editor.tf.setNodes({ alt } as Partial<ImageNode>, {
        at: altState.path,
      });
    }
    setAltState(null);
    editor.tf.focus();
  };

  const over = overLimits(stats);

  return (
    <RichTextMediaContext.Provider value={media}>
      <div
        className={cn(
          "bg-background overflow-hidden rounded-md border",
          invalid && "border-destructive",
          disabled && "opacity-80",
        )}
      >
        <Plate
          editor={editor}
          readOnly={disabled}
          onChange={({ value }) => {
            const next = value as PlateNode[];
            setStats(bodyStats(serializeBody(next).nodes));
            onChange(next);
          }}
        >
          <RichTextToolbar
            onInsertImage={
              renderImagePicker
                ? () => {
                    setPickAt(editor.selection);
                    setPickerOpen(true);
                  }
                : undefined
            }
          />
          <PlateContent
            id={id}
            lang={lang}
            aria-invalid={invalid || undefined}
            placeholder={placeholder}
            className="min-h-72 px-4 py-2 text-sm outline-none [&_[data-slate-placeholder]]:!opacity-60"
          />
        </Plate>
        <div
          className={cn(
            "text-muted-foreground flex flex-wrap justify-end gap-x-3 border-t px-3 py-1 text-[11px] tabular-nums",
            over && "text-destructive",
          )}
        >
          <span>
            {t("editor.rich_text.stats_nodes", {
              count: stats.nodes,
              max: ARTICLE_LIMITS.nodes,
            })}
          </span>
          <span>
            {t("editor.rich_text.stats_size", {
              size: Math.ceil(stats.bytes / 1024),
              max: ARTICLE_LIMITS.bytes / 1024,
            })}
          </span>
        </div>
      </div>
      {renderImagePicker
        ? renderImagePicker({
            open: pickerOpen,
            onOpenChange: setPickerOpen,
            onSelect: (uuid) => {
              setPickerOpen(false);
              setAltState({
                mode: "insert",
                uuid,
                alt: resolveMedia(uuid)?.alt ?? "",
                at: pickAt,
              });
            },
          })
        : null}
      <AppDialog
        open={altState !== null}
        onOpenChange={(open) => {
          if (!open) setAltState(null);
        }}
        title={t("editor.rich_text.image_alt")}
        description={t("editor.rich_text.image_alt_hint")}
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setAltState(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="button" onClick={commitAlt}>
              {altState?.mode === "insert"
                ? t("editor.rich_text.image_insert")
                : t("common.save")}
            </Button>
          </>
        }
      >
        <div className="grid gap-1.5">
          <Label htmlFor="rich-text-alt">
            {t("editor.rich_text.image_alt_label")}
          </Label>
          <Input
            id="rich-text-alt"
            value={altState?.alt ?? ""}
            maxLength={ARTICLE_LIMITS.alt}
            onChange={(e) =>
              setAltState((prev) =>
                prev ? { ...prev, alt: e.target.value } : prev,
              )
            }
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitAlt();
              }
            }}
          />
        </div>
      </AppDialog>
    </RichTextMediaContext.Provider>
  );
}
