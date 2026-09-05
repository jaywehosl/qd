import { createContext, useContext } from 'react';
import type { FormController } from './useFormState';

const FormContext = createContext<FormController<any> | null>(null);

export function FormProvider<T extends object>({ ctl, children }: { ctl: FormController<T>; children: React.ReactNode }) {
  return <FormContext.Provider value={ctl}>{children}</FormContext.Provider>;
}

export function useFormCtl<T extends object = Record<string, unknown>>(): FormController<T> {
  const ctl = useContext(FormContext);
  if (!ctl) throw new Error('useFormCtl must be used inside a <FormProvider>');
  return ctl as FormController<T>;
}
