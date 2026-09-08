import { create } from "zustand";
import {
  listNotifications,
  addNotification,
  markAllNotificationsRead,
  removeNotification,
  clearNotifications,
  isWailsRuntime,
} from "../shared/api/client";

export type NotificationKind = "info" | "success" | "warning" | "error";

export interface AppNotification {
  id: string;
  kind: NotificationKind;
  title: string;
  message?: string;
  /** ISO-время появления. */
  createdAt: string;
  read: boolean;
}

interface NotificationState {
  items: AppNotification[];
  /** Загрузить оповещения из SQLite (backend). Вызывается при старте и по
   *  событию notifications:changed. В веб-режиме без Wails — no-op. */
  hydrate: () => Promise<void>;
  /** Добавить оповещение (новые — сверху). Хранится не более 50 штук.
   *  Оптимистично вставляет локально и персистит в SQLite. */
  push: (n: { kind?: NotificationKind; title: string; message?: string }) => void;
  markAllRead: () => void;
  remove: (id: string) => void;
  clear: () => void;
}

const MAX_ITEMS = 50;

export const useNotificationStore = create<NotificationState>((set) => ({
  items: [],

  hydrate: async () => {
    if (!isWailsRuntime()) return;
    try {
      const rows = await listNotifications();
      set({ items: rows.slice(0, MAX_ITEMS) });
    } catch {
      // Оповещения не критичны — молча игнорируем сбой загрузки.
    }
  },

  push: ({ kind = "info", title, message }) => {
    // Оптимистичная локальная вставка для мгновенной обратной связи
    // (и как единственный источник в веб-режиме без Wails).
    set((state) => ({
      items: [
        {
          id: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
          kind,
          title,
          message,
          createdAt: new Date().toISOString(),
          read: false,
        },
        ...state.items,
      ].slice(0, MAX_ITEMS),
    }));
    // Персист в SQLite. Backend эмитит notifications:changed → hydrate
    // заменит список авторитетными записями (с реальными id), убрав дубль.
    void addNotification(kind, title, message).catch(() => {});
  },

  markAllRead: () => {
    set((state) => ({ items: state.items.map((n) => ({ ...n, read: true })) }));
    void markAllNotificationsRead().catch(() => {});
  },

  remove: (id) => {
    set((state) => ({ items: state.items.filter((n) => n.id !== id) }));
    void removeNotification(id).catch(() => {});
  },

  clear: () => {
    set({ items: [] });
    void clearNotifications().catch(() => {});
  },
}));

/** Удобный хелпер для отправки уведомления из любого места вне React. */
export function notify(n: { kind?: NotificationKind; title: string; message?: string }): void {
  useNotificationStore.getState().push(n);
}

/** Число непрочитанных уведомлений. */
export function selectUnreadCount(state: NotificationState): number {
  return state.items.reduce((acc, n) => acc + (n.read ? 0 : 1), 0);
}
