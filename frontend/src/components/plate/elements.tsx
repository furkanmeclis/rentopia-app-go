"use client";

import { ImageOff, Pencil, Trash2 } from "lucide-react";
import { createContext, useContext } from "react";
import type { TElement } from "platejs";
import {
  PlateElement,
  PlateLeaf,
  useEditorRef,
  useReadOnly,
  useSelected,
  type PlateElementProps,
  type PlateLeafProps,
} from "platejs/react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Library media lookup and the alt editor of the article editor. */
export type RichTextMedia = {
  /** API-owned thumbnail URL and default alt of a library item. */
  resolve: (uuid: string) => { url?: string; alt?: string } | undefined;
  /** Opens the alt dialog for the image at `path`. */
  editAlt?: (element: ImageNode, path: number[]) => void;
};

export const RichTextMediaContext = createContext<RichTextMedia>({
  resolve: () => undefined,
});

export type ImageNode = TElement & { media_uuid?: string; alt?: string };

export function ParagraphElement(props: PlateElementProps) {
  return <PlateElement {...props} as="p" className="my-3 leading-7" />;
}

export function H2Element(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="h2"
      className="mt-7 mb-3 text-2xl font-semibold tracking-tight"
    />
  );
}

export function H3Element(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="h3"
      className="mt-6 mb-2 text-xl font-semibold tracking-tight"
    />
  );
}

export function H4Element(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="h4"
      className="mt-5 mb-2 text-lg font-semibold"
    />
  );
}

export function BlockquoteElement(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="blockquote"
      className="border-primary text-muted-foreground my-4 border-l-2 pl-4 italic"
    />
  );
}

export function BulletedListElement(props: PlateElementProps) {
  return <PlateElement {...props} as="ul" className="my-3 list-disc ps-6" />;
}

export function NumberedListElement(props: PlateElementProps) {
  return <PlateElement {...props} as="ol" className="my-3 list-decimal ps-6" />;
}

export function ListItemElement(props: PlateElementProps) {
  return <PlateElement {...props} as="li" className="my-1" />;
}

export function ListItemContentElement(props: PlateElementProps) {
  return <PlateElement {...props} as="div" />;
}

export function LinkElement(props: PlateElementProps) {
  const url = String((props.element as { url?: unknown }).url ?? "");
  return (
    <PlateElement
      {...props}
      as="a"
      className="text-primary font-medium underline underline-offset-4"
      attributes={{
        ...props.attributes,
        // Never navigates inside the editor; shows the target on hover.
        title: url,
        onClick: (e: React.MouseEvent) => e.preventDefault(),
      }}
    />
  );
}

export function HrElement(props: PlateElementProps) {
  const selected = useSelected();
  return (
    <PlateElement {...props} className="py-3">
      <div contentEditable={false}>
        <hr
          className={cn(
            "bg-border h-0.5 rounded border-none",
            selected && "ring-ring ring-2 ring-offset-2",
          )}
        />
      </div>
      {props.children}
    </PlateElement>
  );
}

export function CodeBlockElement(props: PlateElementProps) {
  return (
    <PlateElement
      {...props}
      as="pre"
      className="bg-muted my-4 overflow-x-auto rounded-md px-4 py-3 font-mono text-sm leading-6"
    />
  );
}

export function CodeLineElement(props: PlateElementProps) {
  return <PlateElement {...props} as="div" />;
}

export function CodeLeaf(props: PlateLeafProps) {
  return (
    <PlateLeaf
      {...props}
      as="code"
      className="bg-muted rounded px-1 py-0.5 font-mono text-[0.9em]"
    />
  );
}

/** Library image (void): thumbnail from the panel's media file route. */
export function ImageElement(props: PlateElementProps) {
  const { t } = useLocale();
  const media = useContext(RichTextMediaContext);
  const editor = useEditorRef();
  const readOnly = useReadOnly();
  const selected = useSelected();
  const element = props.element as ImageNode;
  const uuid = String(element.media_uuid ?? "");
  const item = uuid ? media.resolve(uuid) : undefined;
  const alt = String(element.alt ?? "");
  return (
    <PlateElement {...props} className="my-4">
      <figure
        contentEditable={false}
        className={cn(
          "group relative overflow-hidden rounded-md border",
          selected && "ring-ring ring-2 ring-offset-2",
        )}
      >
        {item?.url ? (
          // eslint-disable-next-line @next/next/no-img-element -- API-owned library stream; not optimizable.
          <img
            src={item.url}
            alt={alt}
            className="max-h-96 w-full object-contain"
          />
        ) : (
          <div className="bg-muted/40 text-muted-foreground flex h-32 flex-col items-center justify-center gap-1 text-xs">
            <ImageOff className="size-5" aria-hidden />
            {t("editor.rich_text.image_missing")}
          </div>
        )}
        <figcaption className="text-muted-foreground flex items-center justify-between gap-2 border-t px-3 py-1.5 text-xs">
          <span className="truncate">
            {alt || t("editor.rich_text.image_no_alt")}
          </span>
          {!readOnly ? (
            <span className="flex shrink-0 gap-1">
              {media.editAlt ? (
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t("editor.rich_text.image_alt")}
                  onClick={() => {
                    const path = editor.api.findPath(element);
                    if (path) media.editAlt?.(element, path);
                  }}
                >
                  <Pencil className="size-3.5" />
                </Button>
              ) : null}
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={t("editor.rich_text.image_remove")}
                onClick={() => {
                  const path = editor.api.findPath(element);
                  if (path) editor.tf.removeNodes({ at: path });
                }}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </span>
          ) : null}
        </figcaption>
      </figure>
      {props.children}
    </PlateElement>
  );
}
