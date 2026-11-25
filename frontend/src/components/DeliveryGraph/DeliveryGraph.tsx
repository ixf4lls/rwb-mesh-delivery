import { Background, Controls, ReactFlow } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { Incident, LogisticsNode, Shipment } from '../../api/deliveryApi';
import type { Order } from '../../api/pvzApi';
import { buildDeliveryGraph } from '../../functions/deliveryGraph';
import { Icon } from '../Icon/Icon';
import { RouteLeg } from '../RouteLeg/RouteLeg';
import { RoutePoint } from '../RoutePoint/RoutePoint';
import styles from './DeliveryGraph.module.css';

const nodeTypes = { routePoint: RoutePoint };
const edgeTypes = { routeLeg: RouteLeg };

type DeliveryGraphProps = {
    shipments?: readonly Shipment[];
    points: ReadonlyMap<string, LogisticsNode>;
    incidents: readonly Incident[];
    order?: Order;
    variant: 'mesh' | 'hub' | 'current' | 'return';
    sellerIds: ReadonlyMap<string, string>;
    onSelectPoint: (id: string) => void;
};

export function DeliveryGraph({ shipments, points, incidents, order, variant, sellerIds, onSelectPoint }: DeliveryGraphProps) {
    const graph = shipments && buildDeliveryGraph({ shipments, points, incidents, order, variant, sellerIds });

    return (
        <div className={styles.graph} aria-label="Логистическая сеть">
            {graph?.nodes.length ? <ReactFlow nodes={graph.nodes} edges={graph.edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
                fitView fitViewOptions={{ padding: 0.16 }} minZoom={0.12} maxZoom={1.8} nodesDraggable={false}
                nodesConnectable={false} edgesFocusable={false} panOnScroll={false} preventScrolling={false}
                onNodeClick={(_, node) => node.data.pointId && onSelectPoint(node.data.pointId)}
                onNodeDoubleClick={(_, node) => node.data.pointId && onSelectPoint(node.data.pointId)}>
                <Background gap={20} size={1.5} color="#dad8e5" />
                <Controls showInteractive={false} />
            </ReactFlow> : <div className={styles.empty}>
                <span><Icon name="routing" /></span>
                <p>Выберите существующий заказ или рассчитайте маршрут для товара.</p>
            </div>}
        </div>
    );
}
