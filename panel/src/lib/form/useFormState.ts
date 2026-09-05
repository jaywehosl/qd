import { useCallback, useState } from 'react';


export type FieldPath = (string | number)[];

type AnyRecord = Record<string, unknown>;

export function getIn(obj: unknown, path: FieldPath): unknown {
  let cur: unknown = obj;
  for (const key of path) {
    if (cur == null || typeof cur !== 'object') return undefined;
    cur = (cur as AnyRecord)[key as string];
  }
  return cur;
}

export function setIn<T>(obj: T, path: FieldPath, value: unknown): T {
  if (path.length === 0) return value as T;
  const [head, ...rest] = path;
  const base: AnyRecord | unknown[] =
    typeof head === 'number'
      ? (Array.isArray(obj) ? [...(obj as unknown[])] : [])
      : { ...((obj as AnyRecord) ?? {}) };
  const child = (base as AnyRecord)[head as string];
  (base as AnyRecord)[head as string] = rest.length === 0 ? value : setIn(child, rest, value);
  return base as T;
}

export function unsetIn<T>(obj: T, path: FieldPath): T {
  if (path.length === 0) return obj;
  if (path.length === 1) {
    const clone: AnyRecord = { ...((obj as AnyRecord) ?? {}) };
    delete clone[path[0] as string];
    return clone as T;
  }
  const [head, ...rest] = path;
  const child = getIn(obj, [head]);
  if (child == null) return obj;
  return setIn(obj, [head], unsetIn(child, rest));
}

export interface FormController<T> {
  values: T;
  get: <V = unknown>(path: FieldPath) => V;
  set: (path: FieldPath, value: unknown) => void;
  unset: (path: FieldPath) => void;
  reset: (next: T) => void;
  setValues: React.Dispatch<React.SetStateAction<T>>;
}

export function useFormState<T extends object>(initial: T | (() => T)): FormController<T> {
  const [values, setValues] = useState<T>(initial);

  const get = useCallback(<V = unknown>(path: FieldPath): V => getIn(values, path) as V, [values]);
  const set = useCallback((path: FieldPath, value: unknown) => {
    setValues((prev) => setIn(prev, path, value));
  }, []);
  const unset = useCallback((path: FieldPath) => {
    setValues((prev) => unsetIn(prev, path));
  }, []);
  const reset = useCallback((next: T) => setValues(next), []);

  return { values, get, set, unset, reset, setValues };
}
