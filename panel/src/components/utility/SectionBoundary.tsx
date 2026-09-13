import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  name: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export default class SectionBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`[${this.props.name}]`, error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div
        style={{
          padding: '16px 18px',
          borderRadius: 10,
          border: '1px solid var(--border-1, #3a3a3a)',
          background: 'var(--bg-2, rgba(255,255,255,.03))',
        }}
      >
        <div style={{ fontWeight: 600, marginBottom: 6 }}>
          This section could not be drawn
        </div>
        <div style={{ fontSize: 13, opacity: 0.75 }}>{error.message}</div>
      </div>
    );
  }
}
