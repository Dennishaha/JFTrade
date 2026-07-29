import { type InjectionKey, type Ref, inject, provide, ref } from "vue";

export interface CommandAction {
  id: string;
  label: string;
  hint?: string;
  group: string;
  keywords?: string[];
  run: () => void;
}

interface CommandPaletteStore {
  open: Ref<boolean>;
  actions: Ref<CommandAction[]>;
  register: (action: CommandAction) => () => void;
  show: () => void;
  hide: () => void;
  toggle: () => void;
}

const paletteKey: InjectionKey<CommandPaletteStore> = Symbol("jftrade-palette");

export function provideCommandPaletteStore(): CommandPaletteStore {
  const open = ref(false);
  const actions = ref<CommandAction[]>([]);

  const store: CommandPaletteStore = {
    open,
    actions,
    register: (action) => {
      actions.value = [
        ...actions.value.filter((a) => a.id !== action.id),
        action,
      ];
      return () => {
        actions.value = actions.value.filter((a) => a.id !== action.id);
      };
    },
    show: () => {
      open.value = true;
    },
    hide: () => {
      open.value = false;
    },
    toggle: () => {
      open.value = !open.value;
    },
  };

  provide(paletteKey, store);
  return store;
}

export function useCommandPalette(): CommandPaletteStore {
  const store = inject(paletteKey);
  if (!store) throw new Error("Command palette store not provided.");
  return store;
}
