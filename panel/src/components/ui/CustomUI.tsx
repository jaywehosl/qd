import React, { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { toast as dsToast } from '@/components/ds/Toast';

// Generate responsive grid styles dynamically at runtime to support all standard breakpoints and layout spans
if (typeof document !== 'undefined') {
  const styleId = 'custom-ui-grid-styles';
  if (!document.getElementById(styleId)) {
    const styleEl = document.createElement('style');
    styleEl.id = styleId;
    let css = `
      .custom-row {
        display: flex;
        flex-wrap: wrap;
        margin-left: -8px;
        margin-right: -8px;
      }
      .custom-col {
        box-sizing: border-box;
        padding-left: 8px;
        padding-right: 8px;
        width: 100%;
        flex: 0 0 100%;
      }
    `;
    for (let i = 1; i <= 24; i++) {
      const pct = (i / 24) * 100;
      css += `.custom-col-${i} { width: ${pct}%; flex: 0 0 ${pct}%; }\n`;
    }
    const breakpoints = {
      xs: 0,
      sm: 576,
      md: 768,
      lg: 992,
      xl: 1200
    };
    for (const [key, val] of Object.entries(breakpoints)) {
      if (val === 0) {
        for (let i = 1; i <= 24; i++) {
          const pct = (i / 24) * 100;
          css += `.custom-col-xs-${i} { width: ${pct}%; flex: 0 0 ${pct}%; }\n`;
        }
      } else {
        css += `@media (min-width: ${val}px) {\n`;
        for (let i = 1; i <= 24; i++) {
          const pct = (i / 24) * 100;
          css += `  .custom-col-${key}-${i} { width: ${pct}%; flex: 0 0 ${pct}%; }\n`;
        }
        css += `}\n`;
      }
    }
    styleEl.textContent = css;
    document.head.appendChild(styleEl);
  }
}


// Card replacement
export function Card({ title, extra, children, actions, className = '', ...props }: any) {
  return (
    <div className={`custom-card ${className}`} {...props}>
      {(title || extra) && (
        <div className="custom-card-header">
          {title && <div className="custom-card-title">{title}</div>}
          {extra && <div className="custom-card-extra">{extra}</div>}
        </div>
      )}
      <div className="custom-card-body">{children}</div>
      {actions && (
        <div className="custom-card-actions">
          {actions.map((act: any, i: number) => (
            <div key={i} className="custom-card-action-item">{act}</div>
          ))}
        </div>
      )}
    </div>
  );
}

// Button replacement
export function Button({ type = 'default', children, className = '', icon, loading, ...props }: any) {
  return (
    <button className={`custom-btn custom-btn-${type} ${className}`} disabled={loading} {...props}>
      {loading && <span className="btn-loading-indicator">⏳</span>}
      {icon && <span className="btn-icon">{icon}</span>}
      {children}
    </button>
  );
}

// Row replacement
export function Row({ gutter = [0, 0], children, className = '', style, ...props }: any) {
  const rowStyle = {
    display: 'flex',
    flexWrap: 'wrap',
    marginLeft: Array.isArray(gutter) ? -gutter[0] / 2 : 0,
    marginRight: Array.isArray(gutter) ? -gutter[0] / 2 : 0,
    rowGap: Array.isArray(gutter) ? gutter[1] : 0,
    ...style,
  };
  return (
    <div className={`custom-row ${className}`} style={rowStyle as any} {...props}>
      {children}
    </div>
  );
}

// Col replacement
export function Col({ span, xs, sm, md, lg, children, className = '', style, ...props }: any) {
  const colClass = [
    'custom-col',
    span ? `custom-col-${span}` : '',
    xs ? `custom-col-xs-${xs}` : '',
    sm ? `custom-col-sm-${sm}` : '',
    md ? `custom-col-md-${md}` : '',
    lg ? `custom-col-lg-${lg}` : '',
    className,
  ]
    .filter(Boolean)
    .join(' ');

  const colStyle = {
    paddingLeft: 8,
    paddingRight: 8,
    ...style,
  };

  return (
    <div className={colClass} style={colStyle} {...props}>
      {children}
    </div>
  );
}

// Space replacement
export function Space({ direction = 'horizontal', align = 'center', size = 8, children, className = '', style, ...props }: any) {
  const spaceStyle = {
    display: 'inline-flex',
    flexDirection: direction === 'vertical' ? 'column' : 'row',
    alignItems: align,
    gap: typeof size === 'number' ? size : 8,
    ...style,
  };
  return (
    <div className={`custom-space ${className}`} style={spaceStyle as any} {...props}>
      {children}
    </div>
  );
}

Space.Compact = function SpaceCompact({ children, className = '', style, ...props }: any) {
  const compactStyle = {
    display: 'flex',
    width: '100%',
    ...style,
  };
  return (
    <div className={`custom-space-compact ant-space-compact ${className}`} style={compactStyle} {...props}>
      {children}
    </div>
  );
};

// Spin replacement (Loader)
export function Spin({ spinning = true, children, description, size = 'default' }: any) {
  if (!spinning) return children || null;
  return (
    <div className={`custom-spin-container size-${size}`}>
      <div className="custom-spin-overlay">
        <svg className="custom-spinner" viewBox="0 0 50 50">
          <circle className="path" cx="25" cy="25" r="20" fill="none" strokeWidth="4" />
        </svg>
        {description && <div className="custom-spin-desc">{description}</div>}
      </div>
      {children && <div className="custom-spin-content-blur">{children}</div>}
    </div>
  );
}

// Tag replacement
export function Tag({ color, children, className = '', ...props }: any) {
  return (
    <span className={`custom-tag color-${color} ${className}`} {...props}>
      {children}
    </span>
  );
}

// Alert replacement
export function Alert({ type = 'info', title, description, showIcon, className = '', ...props }: any) {
  return (
    <div className={`custom-alert type-${type} ${className}`} {...props}>
      {title && <div className="custom-alert-title">{title}</div>}
      {description && <div className="custom-alert-desc">{description}</div>}
    </div>
  );
}


// Custom Modal Dialog
export function Modal({
  open,
  title,
  onClose,
  onCancel,
  onConfirm,
  onOk,
  okText = 'OK',
  cancelText = 'Cancel',
  children,
  confirmLoading = false,
  width,
  style,
}: any) {
  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  if (!open) return null;

  const modalStyle = {
    ...(width ? { width: typeof width === 'number' ? `${width}px` : width } : {}),
    ...style,
  };

  return createPortal(
    <div className="custom-modal-overlay" onClick={onCancel || onClose}>
      <div className="custom-modal-container" onClick={(e) => e.stopPropagation()} style={modalStyle}>
        <div className="custom-modal-header">
          <div className="custom-modal-title">{title}</div>
          <button className="custom-modal-close" onClick={onCancel || onClose}>&times;</button>
        </div>
        <div className="custom-modal-body">{children}</div>
        <div className="custom-modal-footer">
          <Button onClick={onCancel || onClose}>{cancelText}</Button>
          <Button type="primary" onClick={onConfirm || onOk} loading={confirmLoading}>
            {okText}
          </Button>
        </div>
      </div>
    </div>,
    document.body
  );
}

// Custom hook matching Ant Design's Modal.useModal()
Modal.useModal = function useModal() {
  const [modals, setModals] = useState<any[]>([]);

  const confirm = React.useCallback((config: any) => {
    const key = Math.random().toString();
    const newModal = {
      ...config,
      key,
      open: true,
      onCancel: () => {
        setModals((prev) => prev.filter((m) => m.key !== key));
        if (config.onCancel) config.onCancel();
      },
      onConfirm: async () => {
        if (config.onOk) {
          try {
            await config.onOk();
          } catch (e) {
            console.error(e);
          }
        }
        setModals((prev) => prev.filter((m) => m.key !== key));
      },
    };
    setModals((prev) => [...prev, newModal]);
  }, []);

  const modalApi = React.useMemo(() => ({
    confirm,
    info: confirm,
    success: confirm,
    error: confirm,
    warning: confirm,
  }), [confirm]);

  const contextHolder = (
    <>
      {modals.map((m) => (
        <Modal
          key={m.key}
          open={m.open}
          title={m.title}
          okText={m.okText}
          cancelText={m.cancelText}
          onCancel={m.onCancel}
          onConfirm={m.onConfirm}
        >
          {m.content}
        </Modal>
      ))}
    </>
  );

  return [modalApi, contextHolder] as const;
};


// Custom Popover
export function Popover({ title, content, children }: any) {
  const [visible, setVisible] = useState(false);
  return (
    <div
      className="custom-popover-wrapper"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      style={{ position: 'relative', display: 'inline-block' }}
    >
      {children}
      {visible && (
        <div className="custom-popover-content">
          {title && <div className="custom-popover-title">{title}</div>}
          <div className="custom-popover-body">{content}</div>
        </div>
      )}
    </div>
  );
}

// Radio Group buttons
export const Radio = {
  Group: ({ value, onChange, children, style }: any) => {
    return (
      <div className="custom-radio-group" style={style}>
        {React.Children.map(children, (child: any) => {
          if (!child) return null;
          return React.cloneElement(child, {
            checked: child.props.value === value,
            onClick: () => onChange && onChange({ target: { value: child.props.value } }),
          });
        })}
      </div>
    );
  },
  Button: ({ checked, onClick, children }: any) => {
    return (
      <button
        type="button"
        className={`custom-radio-button ${checked ? 'is-checked' : ''}`}
        onClick={onClick}
      >
        {children}
      </button>
    );
  },
};

// Result error pages
export function Result({ status, title, subTitle, extra }: any) {
  return (
    <div className={`custom-result status-${status}`}>
      <div className="custom-result-icon">⚠️</div>
      <h2 className="custom-result-title">{title}</h2>
      <p className="custom-result-subtitle">{subTitle}</p>
      {extra && <div className="custom-result-extra">{extra}</div>}
    </div>
  );
}

// Back top floating button
export function FloatButton() {
  return null;
}

FloatButton.BackTop = function BackTop({ target }: any) {
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const el = target ? target() : window;
    const handleScroll = () => {
      const top = el === window ? window.scrollY : el.scrollTop;
      setVisible(top > 200);
    };
    if (el) {
      el.addEventListener('scroll', handleScroll);
      return () => el.removeEventListener('scroll', handleScroll);
    }
  }, [target]);

  const scrollToTop = () => {
    const el = target ? target() : window;
    if (el) {
      el.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };

  if (!visible) return null;
  return (
    <button className="custom-back-top" onClick={scrollToTop}>
      ↑
    </button>
  );
};

// Pass-through Layout
export const Layout = ({ children, className = '' }: any) => {
  return <div className={`custom-layout ${className}`}>{children}</div>;
};

Layout.Content = ({ children, className = '', id }: any) => {
  return (
    <div id={id} className={`custom-layout-content ${className}`}>
      {children}
    </div>
  );
};

// Pass-through ConfigProvider
export function ConfigProvider({ children }: any) {
  return children;
}

// Toast — delegate to the unified DS toaster (single message system).
export const message = {
  success: (msg: React.ReactNode) => dsToast.success(msg),
  error: (msg: React.ReactNode) => dsToast.error(msg),
  warning: (msg: React.ReactNode) => dsToast.warning(msg),
  info: (msg: React.ReactNode) => dsToast.info(msg),
  config: (_opts?: unknown) => { void _opts; },
  useMessage: () => [message, null] as const,
};

// Tooltip replacement
export function Tooltip({ title, children, placement = 'top' }: any) {
  const [visible, setVisible] = useState(false);
  return (
    <div
      className="custom-tooltip-wrapper"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      style={{ position: 'relative', display: 'inline-block' }}
    >
      {children}
      {visible && title && (
        <div className={`custom-tooltip-content placement-${placement}`}>
          {title}
        </div>
      )}
    </div>
  );
}

// Badge replacement
export function Badge({ status, text, color }: any) {
  return (
    <span className="custom-badge">
      <span
        className={`custom-badge-dot ${status === 'processing' ? 'is-processing' : ''}`}
        style={color ? { backgroundColor: color } : undefined}
      />
      {text && <span className="custom-badge-text">{text}</span>}
    </span>
  );
}

