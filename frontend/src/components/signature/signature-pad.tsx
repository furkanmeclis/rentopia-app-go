"use client";

import { Eraser, Undo2 } from "lucide-react";
import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
} from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** A point: [x, y, pressure 0-1, ms since the first point]. */
export type SignaturePoint = [number, number, number, number];

export type SignatureCapture = {
  width: number;
  height: number;
  strokes: SignaturePoint[][];
  pointCount: number;
};

export type SignaturePadHandle = {
  clear: () => void;
  /** PNG of the canvas (white background), base64 without the data: prefix. */
  toPNG: () => string;
  capture: () => SignatureCapture;
  isEmpty: () => boolean;
};

type SignaturePadProps = {
  ref?: Ref<SignaturePadHandle>;
  /** Canvas height in CSS pixels (the width follows the container). */
  height?: number;
  disabled?: boolean;
  onChange?: (capture: SignatureCapture) => void;
  clearLabel: string;
  undoLabel: string;
  placeholder?: string;
  className?: string;
};

const LINE = 2.4;

/**
 * Signature canvas for tablets and phones: pointer events (finger, pen,
 * mouse) with pressure, kept as strokes of [x, y, pressure, t] points in
 * CSS pixels plus a PNG export. The chrome is shadcn buttons; the drawing
 * surface is a plain canvas.
 */
export function SignaturePad({
  ref,
  height = 200,
  disabled = false,
  onChange,
  clearLabel,
  undoLabel,
  placeholder,
  className,
}: SignaturePadProps) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const strokes = useRef<SignaturePoint[][]>([]);
  const active = useRef<SignaturePoint[] | null>(null);
  const started = useRef<number | null>(null);
  const [width, setWidth] = useState(0);
  const [count, setCount] = useState(0);

  const capture = useCallback(
    (): SignatureCapture => ({
      width,
      height,
      strokes: strokes.current.map((s) =>
        s.map((p) => [...p] as SignaturePoint),
      ),
      pointCount: strokes.current.reduce((n, s) => n + s.length, 0),
    }),
    [height, width],
  );

  const redraw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const ratio = window.devicePixelRatio || 1;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.strokeStyle = "#111827";
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    for (const stroke of strokes.current) {
      for (let i = 1; i < stroke.length; i++) {
        const [x0, y0] = stroke[i - 1];
        const [x1, y1, p] = stroke[i];
        ctx.lineWidth = LINE * (0.5 + (p || 0.5));
        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.stroke();
      }
      if (stroke.length === 1) {
        const [x, y] = stroke[0];
        ctx.beginPath();
        ctx.arc(x, y, LINE / 2, 0, Math.PI * 2);
        ctx.fillStyle = "#111827";
        ctx.fill();
        ctx.fillStyle = "#ffffff";
      }
    }
  }, []);

  // The canvas follows the container width (phones, tablets, rotation).
  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const update = () => setWidth(Math.max(0, Math.floor(wrap.clientWidth)));
    update();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(update);
    observer.observe(wrap);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || width === 0) return;
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.floor(width * ratio);
    canvas.height = Math.floor(height * ratio);
    redraw();
  }, [height, redraw, width]);

  const emit = useCallback(() => {
    setCount(strokes.current.reduce((n, s) => n + s.length, 0));
    onChange?.(capture());
  }, [capture, onChange]);

  const clear = useCallback(() => {
    strokes.current = [];
    active.current = null;
    started.current = null;
    redraw();
    emit();
  }, [emit, redraw]);

  const undo = useCallback(() => {
    strokes.current = strokes.current.slice(0, -1);
    redraw();
    emit();
  }, [emit, redraw]);

  useImperativeHandle(
    ref,
    () => ({
      clear,
      capture,
      isEmpty: () => strokes.current.length === 0,
      toPNG: () => {
        const canvas = canvasRef.current;
        if (!canvas) return "";
        return canvas
          .toDataURL("image/png")
          .replace(/^data:image\/png;base64,/, "");
      },
    }),
    [capture, clear],
  );

  const point = (e: React.PointerEvent<HTMLCanvasElement>): SignaturePoint => {
    const rect = e.currentTarget.getBoundingClientRect();
    const now = performance.now();
    if (started.current === null) started.current = now;
    // Mouse and some fingers report 0 / 0.5: keep a usable default.
    const pressure =
      e.pointerType === "mouse" || !e.pressure ? 0.5 : Math.min(1, e.pressure);
    return [
      Math.round((e.clientX - rect.left) * 10) / 10,
      Math.round((e.clientY - rect.top) * 10) / 10,
      Math.round(pressure * 1000) / 1000,
      Math.round(now - started.current),
    ];
  };

  return (
    <div className={cn("space-y-2", className)}>
      <div
        ref={wrapRef}
        className="bg-background relative w-full overflow-hidden rounded-md border"
        style={{ height }}
      >
        <canvas
          ref={canvasRef}
          aria-label={placeholder}
          role="img"
          className={cn(
            "block touch-none select-none",
            disabled ? "cursor-not-allowed opacity-60" : "cursor-crosshair",
          )}
          style={{ width, height }}
          onPointerDown={(e) => {
            if (disabled) return;
            e.currentTarget.setPointerCapture(e.pointerId);
            active.current = [point(e)];
            strokes.current = [...strokes.current, active.current];
            redraw();
          }}
          onPointerMove={(e) => {
            if (disabled || !active.current) return;
            const events =
              typeof e.nativeEvent.getCoalescedEvents === "function"
                ? e.nativeEvent.getCoalescedEvents()
                : [];
            if (events.length > 1) {
              for (const ev of events) {
                active.current.push(
                  point({
                    ...e,
                    clientX: ev.clientX,
                    clientY: ev.clientY,
                    pressure: ev.pressure,
                    pointerType: ev.pointerType,
                  } as React.PointerEvent<HTMLCanvasElement>),
                );
              }
            } else {
              active.current.push(point(e));
            }
            redraw();
          }}
          onPointerUp={() => {
            if (!active.current) return;
            active.current = null;
            emit();
          }}
          onPointerCancel={() => {
            active.current = null;
            emit();
          }}
        />
        {count === 0 && placeholder ? (
          <span className="text-muted-foreground pointer-events-none absolute inset-x-0 bottom-3 text-center text-sm">
            {placeholder}
          </span>
        ) : null}
      </div>
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={undo}
          disabled={disabled || count === 0}
        >
          <Undo2 className="size-4" />
          {undoLabel}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={clear}
          disabled={disabled || count === 0}
        >
          <Eraser className="size-4" />
          {clearLabel}
        </Button>
      </div>
    </div>
  );
}
