import { Handle, Position } from '@xyflow/react';
import type { NodeProps } from '@xyflow/react';
import type { GraphNode } from '../../functions/deliveryGraph';
import styles from './RoutePoint.module.css';

export function RoutePoint({ data, selected, sourcePosition, targetPosition }: NodeProps<GraphNode>) {
    return (
        <div className={styles.point} data-state={data.state} data-selected={selected}>
            <Handle type="target" position={targetPosition ?? Position.Left} className={styles.handle} />
            <small>{data.caption}</small>
            <strong>{data.title}</strong>
            {data.state === 'current' && <span>Последний подтверждённый узел</span>}
            {data.state === 'unavailable' && <span>Недоступен</span>}
            <Handle type="source" position={sourcePosition ?? Position.Right} className={styles.handle} />
        </div>
    );
}
