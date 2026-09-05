import {
  createContext, useCallback, useContext, useEffect, useMemo, useRef,
  useState, createElement, type ReactNode,
} from 'react';

export interface EditorDescriptor {
  id: string;
  dirty: boolean;
  restartNeeded: boolean;
  busy: boolean;
  saveLabel: string;
  restartLabel: string;
  restartKind: 'panel' | 'xray';
  save: () => void | Promise<void>;
  restart: () => void | Promise<void>;
  discardLabel?: string;
  discard?: () => void | Promise<void>;
}

export interface HeaderActionsState {
  dirty: boolean;
  restartNeeded: boolean;
  busy: boolean;
  saveText: string;
  restartText: string;
  discardText: string;
  onSave: () => void;
  onRestart: () => void;
  onDiscard: (() => void) | null;
}

interface HeaderActionsContextValue {
  editors: Record<string, EditorDescriptor>;
  register: (d: EditorDescriptor) => void;
  unregister: (id: string) => void;
}

const HeaderActionsContext = createContext<HeaderActionsContextValue | null>(null);

export function HeaderActionsProvider({ children }: { children: ReactNode }) {
  const [editors, setEditors] = useState<Record<string, EditorDescriptor>>({});

  const register = useCallback((d: EditorDescriptor) => {
    setEditors((prev) => {
      const ex = prev[d.id];
      if (
        ex
        && ex.dirty === d.dirty
        && ex.restartNeeded === d.restartNeeded
        && ex.busy === d.busy
        && ex.saveLabel === d.saveLabel
        && ex.restartLabel === d.restartLabel
        && ex.restartKind === d.restartKind
        && ex.save === d.save
        && ex.restart === d.restart
      ) {
        return prev;
      }
      return { ...prev, [d.id]: d };
    });
  }, []);

  const unregister = useCallback((id: string) => {
    setEditors((prev) => {
      if (!(id in prev)) return prev;
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }, []);

  const value = useMemo<HeaderActionsContextValue>(
    () => ({ editors, register, unregister }),
    [editors, register, unregister],
  );

  return createElement(HeaderActionsContext.Provider, { value }, children);
}

function useRegistry(): HeaderActionsContextValue {
  const ctx = useContext(HeaderActionsContext);
  if (!ctx) throw new Error('useHeaderActions must be used within a HeaderActionsProvider');
  return ctx;
}

export function useHeaderActions(): HeaderActionsState | null {
  const { editors } = useRegistry();
  return useMemo(() => {
    const list = Object.values(editors);
    const dirtyEditors = list.filter((e) => e.dirty);
    const restartEditors = list.filter((e) => e.restartNeeded);
    if (dirtyEditors.length === 0 && restartEditors.length === 0) return null;

    const target = restartEditors.find((e) => e.restartKind === 'panel') ?? restartEditors[0];

    return {
      dirty: dirtyEditors.length > 0,
      restartNeeded: restartEditors.length > 0,
      busy: list.some((e) => e.busy),
      saveText: dirtyEditors[0]?.saveLabel ?? '',
      restartText: target?.restartLabel ?? '',
      discardText: dirtyEditors.find((e) => e.discard)?.discardLabel ?? '',
      onSave: () => { dirtyEditors.forEach((e) => { void e.save(); }); },
      onRestart: () => { void target?.restart(); },
      onDiscard: dirtyEditors.some((e) => e.discard)
        ? () => { dirtyEditors.forEach((e) => { void e.discard?.(); }); }
        : null,
    };
  }, [editors]);
}

export function useRegisterEditor(desc: EditorDescriptor): void {
  const { register, unregister } = useRegistry();

  const ref = useRef(desc);
  ref.current = desc;

  const save = useCallback(() => ref.current.save(), []);
  const restart = useCallback(() => ref.current.restart(), []);
  const discard = useCallback(() => ref.current.discard?.(), []);
  const hasDiscard = !!desc.discard;

  const { id, dirty, restartNeeded, busy, saveLabel, restartLabel, restartKind, discardLabel } = desc;
  useEffect(() => {
    register({
      id, dirty, restartNeeded, busy, saveLabel, restartLabel, restartKind, save, restart,
      discardLabel, discard: hasDiscard ? discard : undefined,
    });
  }, [register, id, dirty, restartNeeded, busy, saveLabel, restartLabel, restartKind,
      save, restart, discardLabel, hasDiscard, discard]);

  useEffect(() => () => unregister(id), [unregister, id]);
}
