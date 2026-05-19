import {
  type InjectionKey,
  type Ref,
  computed,
  inject,
  provide,
  ref,
} from "vue";

export type NotificationLevel = "info" | "success" | "warn" | "error";

export interface NotificationItem {
  id: string;
  level: NotificationLevel;
  title: string;
  message?: string;
  source?: string;
  at: string;
  read: boolean;
}

interface NotificationsStore {
  items: Ref<NotificationItem[]>;
  unreadCount: Ref<number>;
  push: (
    input: Omit<NotificationItem, "id" | "at" | "read"> & { at?: string },
  ) => void;
  markAllRead: () => void;
  clear: () => void;
  remove: (id: string) => void;
}

const notificationsKey: InjectionKey<NotificationsStore> = Symbol(
  "jftrade-notifications",
);

const MAX_ITEMS = 100;

function generateId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `n-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export function provideNotificationsStore(): NotificationsStore {
  const items = ref<NotificationItem[]>([]);
  const unreadCount = computed(
    () => items.value.filter((item) => !item.read).length,
  );

  const store: NotificationsStore = {
    items,
    unreadCount,
    push: ({ at, ...input }) => {
      const next: NotificationItem = {
        id: generateId(),
        at: at ?? new Date().toISOString(),
        read: false,
        ...input,
      };
      items.value = [next, ...items.value].slice(0, MAX_ITEMS);
    },
    markAllRead: () => {
      items.value = items.value.map((item) => ({ ...item, read: true }));
    },
    clear: () => {
      items.value = [];
    },
    remove: (id) => {
      items.value = items.value.filter((item) => item.id !== id);
    },
  };

  provide(notificationsKey, store);
  return store;
}

export function useNotifications(): NotificationsStore {
  const store = inject(notificationsKey);
  if (!store) {
    throw new Error("Notifications store not provided.");
  }
  return store;
}
