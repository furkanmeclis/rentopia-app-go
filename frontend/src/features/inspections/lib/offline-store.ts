/**
 * Device-side storage of the mobile handover / return wizard: the photo
 * upload queue (with the image bytes) and unsynced draft fields survive a
 * reload or a lost connection. IndexedDB when the browser allows it, an
 * in-memory map otherwise (private mode, blocked storage): every access is
 * wrapped so the wizard keeps working without persistence.
 */

export type QueueStatus =
  "queued" | "uploading" | "completing" | "done" | "failed";

export type QueueItem = {
  /** Also the API `client_ref` (idempotent retries). */
  id: string;
  slug: string;
  inspectionUuid: string;
  slot: string;
  damageUuid?: string;
  name: string;
  type: string;
  size: number;
  capturedAt?: string;
  blob: Blob;
  status: QueueStatus;
  error?: string;
  photoUuid?: string;
  uploadPath?: string;
  createdAt: number;
};

export type DraftPatch = Record<string, unknown>;

const DB_NAME = "rentopia-inspections";
const DB_VERSION = 1;
const QUEUE = "queue";
const DRAFTS = "drafts";

const memoryQueue = new Map<string, QueueItem>();
const memoryDrafts = new Map<string, DraftPatch>();

let dbPromise: Promise<IDBDatabase | null> | null = null;

function openDb(): Promise<IDBDatabase | null> {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve) => {
    try {
      if (typeof indexedDB === "undefined") {
        resolve(null);
        return;
      }
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(QUEUE)) {
          db.createObjectStore(QUEUE, { keyPath: "id" });
        }
        if (!db.objectStoreNames.contains(DRAFTS)) {
          db.createObjectStore(DRAFTS);
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(null);
      req.onblocked = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
  return dbPromise;
}

function run<T>(
  store: string,
  mode: IDBTransactionMode,
  fn: (s: IDBObjectStore) => IDBRequest<T>,
): Promise<T | undefined> {
  return openDb().then(
    (db) =>
      new Promise<T | undefined>((resolve) => {
        if (!db) {
          resolve(undefined);
          return;
        }
        try {
          const tx = db.transaction(store, mode);
          const req = fn(tx.objectStore(store));
          req.onsuccess = () => resolve(req.result);
          req.onerror = () => resolve(undefined);
        } catch {
          resolve(undefined);
        }
      }),
  );
}

async function persistent() {
  return (await openDb()) !== null;
}

export async function putItem(item: QueueItem) {
  memoryQueue.set(item.id, item);
  if (await persistent()) await run(QUEUE, "readwrite", (s) => s.put(item));
}

export async function deleteItem(id: string) {
  memoryQueue.delete(id);
  if (await persistent()) await run(QUEUE, "readwrite", (s) => s.delete(id));
}

/** Queue items of one form (oldest first). */
export async function listItems(inspectionUuid: string): Promise<QueueItem[]> {
  let items: QueueItem[] = [];
  if (await persistent()) {
    const all = await run<QueueItem[]>(QUEUE, "readonly", (s) => s.getAll());
    items = all ?? [];
    for (const item of items) memoryQueue.set(item.id, item);
  } else {
    items = [...memoryQueue.values()];
  }
  return items
    .filter((item) => item.inspectionUuid === inspectionUuid)
    .sort((a, b) => a.createdAt - b.createdAt);
}

export async function saveDraft(inspectionUuid: string, patch: DraftPatch) {
  memoryDrafts.set(inspectionUuid, patch);
  if (await persistent()) {
    await run(DRAFTS, "readwrite", (s) => s.put(patch, inspectionUuid));
  }
}

export async function loadDraft(
  inspectionUuid: string,
): Promise<DraftPatch | undefined> {
  if (await persistent()) {
    const value = await run<DraftPatch>(DRAFTS, "readonly", (s) =>
      s.get(inspectionUuid),
    );
    if (value) return value;
  }
  return memoryDrafts.get(inspectionUuid);
}

export async function clearDraft(inspectionUuid: string) {
  memoryDrafts.delete(inspectionUuid);
  if (await persistent()) {
    await run(DRAFTS, "readwrite", (s) => s.delete(inspectionUuid));
  }
}

/** A random id for the device queue (also the API idempotency key). */
export function newClientRef() {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }
}
