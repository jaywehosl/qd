import { forwardRef } from 'react';
import type { InputHTMLAttributes, TextareaHTMLAttributes, ReactNode } from 'react';

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className = '', ...rest },
  ref,
) {
  return <input ref={ref} className={['ds-input', className].filter(Boolean).join(' ')} {...rest} />;
});

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { className = '', ...rest },
  ref,
) {
  return <textarea ref={ref} className={['ds-input', className].filter(Boolean).join(' ')} {...rest} />;
});

export interface FieldProps {
  label?: ReactNode;
  error?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
}

/** Label + control + error-slot wrapper for forms. */
export function Field({ label, error, htmlFor, children, className = '' }: FieldProps) {
  return (
    <div className={['ds-field', className].filter(Boolean).join(' ')}>
      {label != null && (
        <label className="ds-field__label" htmlFor={htmlFor}>
          {label}
        </label>
      )}
      {children}
      {error != null && <span className="ds-field__error">{error}</span>}
    </div>
  );
}
