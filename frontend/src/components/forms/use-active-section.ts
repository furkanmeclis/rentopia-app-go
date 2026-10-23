"use client";

import { useEffect, useMemo, useState } from "react";

/** Matches FormSection scroll-mt-24 + sticky app header. */
const ACTIVATION_OFFSET = 140;
/** Inside an overlay there is no app header; a short lead is enough. */
const NESTED_ACTIVATION_OFFSET = 48;
const SCROLL_BOTTOM_THRESHOLD = 8;

/**
 * Where the sections scroll: the window (form pages) or the nearest
 * scrollable ancestor (forms embedded in a drawer / dialog body).
 */
export type ActiveSectionScroll = "window" | "nearest";

export function findScrollParent(el: HTMLElement): HTMLElement | null {
  let node = el.parentElement;
  while (node && node !== document.body) {
    const { overflowY } = window.getComputedStyle(node);
    if (overflowY === "auto" || overflowY === "scroll") return node;
    node = node.parentElement;
  }
  return null;
}

function resolveActiveSection(
  elements: HTMLElement[],
  container: HTMLElement | null,
): string {
  if (elements.length === 0) return "";

  const atBottom = container
    ? container.scrollHeight > container.clientHeight &&
      container.scrollTop + container.clientHeight >=
        container.scrollHeight - SCROLL_BOTTOM_THRESHOLD
    : window.scrollY + window.innerHeight >=
      document.documentElement.scrollHeight - SCROLL_BOTTOM_THRESHOLD;

  if (atBottom) {
    return elements[elements.length - 1].id;
  }

  const threshold = container
    ? container.getBoundingClientRect().top + NESTED_ACTIVATION_OFFSET
    : ACTIVATION_OFFSET;

  let activeId = elements[0].id;

  for (const el of elements) {
    if (el.getBoundingClientRect().top <= threshold) {
      activeId = el.id;
    }
  }

  return activeId;
}

/**
 * Tracks which section id is currently active while scrolling.
 * Used by FormNav for sidebar / pill highlighting.
 */
export function useActiveSection(
  sectionIds: string[],
  enabled = true,
  scroll: ActiveSectionScroll = "window",
) {
  const idsKey = sectionIds.join("|");
  const ids = useMemo(() => idsKey.split("|").filter(Boolean), [idsKey]);
  const [activeId, setActiveId] = useState(ids[0] ?? "");

  const resolvedActiveId = ids.includes(activeId) ? activeId : (ids[0] ?? "");

  useEffect(() => {
    if (!enabled || ids.length === 0) return;

    const elements = ids
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => Boolean(el));

    if (elements.length === 0) return;

    const container =
      scroll === "nearest" ? findScrollParent(elements[0]) : null;
    const target: HTMLElement | Window = container ?? window;

    let frame = 0;

    const update = () => {
      setActiveId(resolveActiveSection(elements, container));
    };

    const scheduleUpdate = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };

    update();
    target.addEventListener("scroll", scheduleUpdate, { passive: true });
    window.addEventListener("resize", scheduleUpdate);

    const resizeObserver = new ResizeObserver(scheduleUpdate);
    resizeObserver.observe(container ?? document.documentElement);

    return () => {
      cancelAnimationFrame(frame);
      target.removeEventListener("scroll", scheduleUpdate);
      window.removeEventListener("resize", scheduleUpdate);
      resizeObserver.disconnect();
    };
  }, [enabled, ids, scroll]);

  return resolvedActiveId;
}
