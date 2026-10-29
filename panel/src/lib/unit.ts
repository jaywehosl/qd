import i18next from 'i18next';

export const unit = (name: string): string => i18next.t(`unit.${name}`, { defaultValue: name });
