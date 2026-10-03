import React, { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { toast as dsToast } from '@/components/ds/Toast';

export function Button({ type = 'default', children, className = '', icon, loading, ...props }: any) {
  return (
    <button className={`custom-btn custom-btn-${type} ${className}`} disabled={loading} {...props}>
      {loading && <span className="btn-loading-indicator">⏳</span>}
      {icon && <span className="btn-icon">{icon}</span>}
      {children}
    </button>
  );
}

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

export function Tag({ color, children, className = '', ...props }: any) {
  return (
    <span className={`custom-tag color-${color} ${className}`} {...props}>
      {children}
    </span>
  );
}

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


export const message = {
  success: (msg: React.ReactNode) => dsToast.success(msg),
  error: (msg: React.ReactNode) => dsToast.error(msg),
  warning: (msg: React.ReactNode) => dsToast.warning(msg),
  info: (msg: React.ReactNode) => dsToast.info(msg),
  config: (_opts?: unknown) => { void _opts; },
  useMessage: () => [message, null] as const,
};

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
