import type React from 'react';

import { Tag } from '@/components/ds';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
interface NodeSelectProps {
  nodes: NodeRecord[];
  value: number | null;
  onChange: (id: number) => void;
  empty?: string;
}

/** Row of node pills, one active. Drives which node a modal reads from. */
export default function NodeSelect({ nodes, value, onChange, empty }: NodeSelectProps) {
  if (nodes.length === 0) return <div className="node-select__empty">{empty}</div>;
  return (
    <div className="node-select" style={{ '--n': Math.max(nodes.length, 1) } as React.CSSProperties}>
      {nodes.map((n, i) => {
        const active = n.id === value;
        return (
          <Tag
            key={n.id}
            tone={active ? 'primary' : 'neutral'}
            className={`node-select__pill${active ? ' is-active' : ''}`}
            style={{ '--i': i } as React.CSSProperties}
            onClick={() => onChange(n.id)}
          >
            {n.name || n.address}
          </Tag>
        );
      })}
    </div>
  );
}
