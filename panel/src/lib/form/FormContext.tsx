import { createContext, useContext } from 'react';
import type { FormController } from './useFormState';

/**
 * Shares a useFormState controller down the tree so deeply-nested field
 * components (protocol/transport/security forms) read & write the form without
 * prop-drilling — the controlled-state replacement for antd's Form context.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const FormContext = createContext<FormController<any> | null>(null);

export function FormProvider<T extends object>({ ctl, children }: { ctl: FormController<T>; children: React.ReactNode }) {
  return <FormContext.Provider value={ctl}>{children}</FormContext.Provider>;
}

/** Read the form controller from context. Throws if used outside a FormProvider. */
export function useFormCtl<T extends object = Record<string, unknown>>(): FormController<T> {
  const ctl = useContext(FormContext);
  if (!ctl) throw new Error('useFormCtl must be used inside a <FormProvider>');
  return ctl as FormController<T>;
}
