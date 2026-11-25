import { BaseEdge, EdgeLabelRenderer, getStraightPath } from '@xyflow/react';
import type { EdgeProps } from '@xyflow/react';
import styles from './RouteLeg.module.css';

export function RouteLeg({ id, sourceX, sourceY, targetX, targetY, markerEnd, style, label }: EdgeProps) {
    const [path, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY });
    const horizontal = Math.abs(targetX - sourceX) > Math.abs(targetY - sourceY);

    return (
        <>
            <BaseEdge id={id} path={path} markerEnd={markerEnd} style={style} />
            <EdgeLabelRenderer>
                <span className={styles.label} data-horizontal={horizontal}
                    style={{ left: labelX, top: labelY, color: style?.stroke }}>{label}</span>
            </EdgeLabelRenderer>
        </>
    );
}
