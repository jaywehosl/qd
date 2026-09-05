import * as RSwitch from '@radix-ui/react-switch';

export interface SwitchProps {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
  id?: string;
  'aria-label'?: string;
}

export function Switch({ checked, onChange, disabled, id, ...rest }: SwitchProps) {
  return (
    <RSwitch.Root
      id={id}
      className="ds-switch"
      checked={checked}
      onCheckedChange={onChange}
      disabled={disabled}
      {...rest}
    >
      <RSwitch.Thumb className="ds-switch__thumb" />
    </RSwitch.Root>
  );
}
