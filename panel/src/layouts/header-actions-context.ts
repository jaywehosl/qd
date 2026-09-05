import {
  createContext, useCallback, useContext, useEffect, useMemo, useRef,
  useState, createElement, type ReactNode,
} from 'react';

/**
 * Global registry of "editor" subsystems (panel settings, Xray core, …) that
 * each own a draft with unsaved edits and a post-save restart step.
 *
 * The header surfaces ONE aggregated Save / Restart pair built from every
 * registered editor — NOT a page-scoped pair. This is deliberate: an editor's
 * draft lives in an always-mounted controller (SettingsController /
 * XrayController), so unsaved edits survive navigating away from its page. The
 * header must therefore keep showing Save while ANY editor is dirty, on every
 * page — otherwise you'd lose sight of pending changes the moment you leave.
 *
 * Aggregation rules (per product decision):
 *   • Save  — shown when ANY editor is dirty; clicking saves ALL dirty editors.
 *   • Restart — shown when ANY editor needs a restart (and nothing is dirty).
 *     A 'panel' restart is a superset (it reloads the frontend AND restarts the
 *     core), so when several restarts are pending we run the panel one and let
 *     it cover the rest.
 */
export interface EditorDescriptor {
  /** stable subsystem id, e.g. 'settings' | 'xray' */
  id: string;
  /** unsaved edits exist */
  dirty: boolean;
  /** a save succeeded and a restart is now appropriate */
  restartNeeded: boolean;
  /** an action is in flight */
  busy: boolean;
  saveLabel: string;
  restartLabel: string;
  /** 'panel' reloads the whole frontend (superset); 'xray' restarts core only */
  restartKind: 'panel' | 'xray';
  save: () => void | Promise<void>;
  restart: () => void | Promise<void>;
  /** Optional secondary action rendered left of Save — throws the edits away
   *  instead of persisting them. Only editors whose draft outlives the page
   *  need it. */
  discardLabel?: string;
  discard?: () => void | Promise<void>;
}

/** The aggregate the header consumes (shape kept stable for AppSidebar). */
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
        return prev; // no-op: avoids a re-render storm
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

/**
 * Read the aggregated header actions. Returns null when nothing is pending
 * (no Save and no Restart to show).
 */
export function useHeaderActions(): HeaderActionsState | null {
  const { editors } = useRegistry();
  return useMemo(() => {
    const list = Object.values(editors);
    const dirtyEditors = list.filter((e) => e.dirty);
    const restartEditors = list.filter((e) => e.restartNeeded);
    if (dirtyEditors.length === 0 && restartEditors.length === 0) return null;

    // Prefer a 'panel' restart — it's the superset (reloads frontend + core).
    const target = restartEditors.find((e) => e.restartKind === 'panel') ?? restartEditors[0];

    return {
      dirty: dirtyEditors.length > 0,
      restartNeeded: restartEditors.length > 0,
      busy: list.some((e) => e.busy),
      saveText: dirtyEditors[0]?.saveLabel ?? '',
      restartText: target?.restartLabel ?? '',
      discardText: dirtyEditors.find((e) => e.discard)?.discardLabel ?? '',
      // One Save click persists EVERY dirty editor (all pages' changes).
      onSave: () => { dirtyEditors.forEach((e) => { void e.save(); }); },
      onRestart: () => { void target?.restart(); },
      onDiscard: dirtyEditors.some((e) => e.discard)
        ? () => { dirtyEditors.forEach((e) => { void e.discard?.(); }); }
        : null,
    };
  }, [editors]);
}

/**
 * Register an editor subsystem for as long as the calling controller is
 * mounted (controllers live at the layout level, so this persists across page
 * navigation). Callbacks whose identity changes every render are fine — we keep
 * the latest descriptor in a ref and register STABLE save/restart wrappers, so
 * the registry only churns when the primitive fields actually change.
 */
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

  // clear on unmount only
  useEffect(() => () => unregister(id), [unregister, id]);
}
