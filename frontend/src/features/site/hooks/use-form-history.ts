"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { FieldValues, UseFormReturn } from "react-hook-form";

const clone = <T>(value: T): T =>
  typeof structuredClone === "function"
    ? structuredClone(value)
    : (JSON.parse(JSON.stringify(value)) as T);

const same = (a: unknown, b: unknown) =>
  JSON.stringify(a) === JSON.stringify(b);

/**
 * Undo / redo over the values of one form: edits are grouped into a
 * snapshot after a short pause, at most `limit` steps are kept, and a new
 * edit drops the redo stack. Undoing resets the form to the snapshot while
 * keeping its default values, so dirty tracking stays correct.
 */
export function useFormHistory<T extends FieldValues>(
  form: UseFormReturn<T>,
  { limit = 50, delay = 400 }: { limit?: number; delay?: number } = {},
) {
  const past = useRef<T[]>([]);
  const future = useRef<T[]>([]);
  const current = useRef<T>(clone(form.getValues()));
  const timer = useRef<number | undefined>(undefined);
  const applying = useRef(false);
  const [counts, setCounts] = useState({ past: 0, future: 0, pending: false });
  const rerender = () =>
    setCounts({
      past: past.current.length,
      future: future.current.length,
      pending: timer.current !== undefined,
    });

  const commit = useCallback(() => {
    window.clearTimeout(timer.current);
    timer.current = undefined;
    const next = clone(form.getValues());
    if (same(next, current.current)) {
      rerender();
      return;
    }
    past.current.push(current.current);
    if (past.current.length > limit) past.current.shift();
    future.current = [];
    current.current = next;
    rerender();
  }, [form, limit]);

  useEffect(() => {
    const subscription = form.watch(() => {
      if (applying.current) return;
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(commit, delay);
      setCounts((c) => (c.pending ? c : { ...c, pending: true }));
    });
    return () => {
      subscription.unsubscribe();
      window.clearTimeout(timer.current);
    };
  }, [form, commit, delay]);

  const apply = useCallback(
    (values: T) => {
      current.current = values;
      applying.current = true;
      try {
        form.reset(clone(values), {
          keepDefaultValues: true,
          keepErrors: false,
          keepSubmitCount: true,
        });
      } finally {
        applying.current = false;
      }
      rerender();
    },
    [form],
  );

  const undo = useCallback(() => {
    if (timer.current !== undefined) commit();
    const previous = past.current.pop();
    if (!previous) return;
    future.current.push(current.current);
    apply(previous);
  }, [apply, commit]);

  const redo = useCallback(() => {
    if (timer.current !== undefined) commit();
    const next = future.current.pop();
    if (!next) return;
    past.current.push(current.current);
    apply(next);
  }, [apply, commit]);

  return {
    undo,
    redo,
    canUndo: counts.past > 0 || counts.pending,
    canRedo: counts.future > 0 && !counts.pending,
  };
}

/**
 * Cmd/Ctrl+Z and Shift+Cmd/Ctrl+Z (or Ctrl+Y) for `useFormHistory`,
 * ignored while a dialog is open.
 */
export function useUndoShortcuts(
  enabled: boolean,
  undo: () => void,
  redo: () => void,
) {
  useEffect(() => {
    if (!enabled) return;
    const onKey = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.altKey) return;
      const key = event.key.toLowerCase();
      const target = event.target as HTMLElement | null;
      if (target?.closest?.('[role="dialog"],[role="alertdialog"]')) return;
      if (key === "z" && !event.shiftKey) {
        event.preventDefault();
        undo();
      } else if ((key === "z" && event.shiftKey) || key === "y") {
        event.preventDefault();
        redo();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [enabled, undo, redo]);
}
