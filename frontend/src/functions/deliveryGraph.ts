import { MarkerType, Position } from '@xyflow/react';
import type { Edge, Node } from '@xyflow/react';
import type { Incident, LogisticsNode, Shipment } from '../api/deliveryApi';
import type { Order } from '../api/pvzApi';
import { isBlockedSegment, nodeKindNames, pointName, segmentDistance, segmentDuration } from './deliveryRoutes';
import { formatMoney } from './money';

export type GraphNode = Node<{ pointId?: string; title: string; caption: string; state: 'current' | 'unavailable' | 'normal' }, 'routePoint'>;
type GraphRoute = {
    shipments: readonly Shipment[];
    points: ReadonlyMap<string, LogisticsNode>;
    order?: Order;
    incidents: readonly Incident[];
    variant: 'mesh' | 'hub' | 'current' | 'return';
    sellerIds: ReadonlyMap<string, string>;
};

const colors = { mesh: '#5932ea', hub: '#52647b', current: '#5932ea', return: '#216957' };

function facing(point: GraphNode['position'], other: GraphNode['position'], fallback: Position) {
    if (point.x !== other.x) return other.x > point.x ? Position.Right : Position.Left;
    if (point.y !== other.y) return other.y > point.y ? Position.Bottom : Position.Top;
    return fallback;
}

export function buildDeliveryGraph({ shipments, points, order, incidents, variant, sellerIds }: GraphRoute) {
    const nodes: GraphNode[] = [];
    const edges: Edge[] = [];
    let laneTop = 0;

    shipments.forEach((shipment) => {
        if (variant !== 'return' && shipment.fromNodeId === shipment.toNodeId
            && shipment.segments.every((segment) => segment.fromNodeId === segment.toNodeId)) {
            const pointId = shipment.toNodeId;
            const id = `pickup:${pointId}`;
            const existing = nodes.find((node) => node.id === id);
            const live = order?.shipments.find((item) => item.id === shipment.id);
            const blocked = points.get(pointId)?.active === false || incidents.some((incident) => (
                incident.status !== 'resolved' && incident.failureTarget.nodeId === pointId
            ));
            const current = live?.lastKnownNodeId === pointId;
            if (existing) {
                if (current && existing.data.state !== 'unavailable') existing.data.state = 'current';
                return;
            }
            nodes.push({ id, type: 'routePoint', position: { x: 0, y: laneTop },
                data: { pointId, title: pointName(points, pointId), caption: 'Товар доступен в ПВЗ выдачи',
                    state: blocked ? 'unavailable' : current ? 'current' : 'normal' } });
            laneTop += 220;
            return;
        }
        const laneNodes: GraphNode[] = [];
        const live = order?.shipments.find((item) => item.id === shipment.id);
        const activeIndex = shipment.segments.findIndex(({ id }) => id === live?.activeSegmentId);
        const nextIndex = shipment.segments.findIndex(({ id }) => id === live?.nextSegmentId);

        function addNode(id: string, title: string, caption: string, pointId?: string) {
            const blocked = pointId !== undefined && (points.get(pointId)?.active === false || incidents.some((incident) => (
                incident.status !== 'resolved' && incident.failureTarget.nodeId === pointId
            )));
            const current = pointId !== undefined && pointId === live?.lastKnownNodeId;
            laneNodes.push({ id, type: 'routePoint', position: { x: 0, y: 0 },
                data: { pointId, title, caption, state: blocked ? 'unavailable' : current ? 'current' : 'normal' } });
        }

        function addStop(id: string, nodeId: string) {
            const point = points.get(nodeId);
            const isPickup = variant !== 'return' && nodeId === shipment.toNodeId
                && (point?.kind === 'storage_pvz' || point?.kind === 'pickup_only');
            addNode(id, pointName(points, nodeId), isPickup ? 'ПВЗ выдачи' : point ? nodeKindNames[point.kind] : 'Узел маршрута', nodeId);
        }

        const first = shipment.segments[0];
        if (!first) return;

        const startId = `${shipment.id}:start`;
        const sellers = [...new Set(shipment.contents.map(({ productId }) => sellerIds.get(productId)).filter(Boolean))];
        if (variant !== 'return' && shipment.fromNodeId !== shipment.toNodeId && sellers.length > 0) {
            const id = `${shipment.id}:seller`;
            addNode(id, sellers.join(', '), 'Продавец');
            edges.push({ id: `${id}:link`, source: id, target: startId, type: 'straight', style: { strokeDasharray: '4 5' }, label: 'Товар' });
        }
        addStop(startId, first.fromNodeId);
        let lastNodeId = first.fromNodeId;
        let lastGraphId = startId;

        shipment.segments.forEach((segment, index) => {
            if (segment.fromNodeId === segment.toNodeId) return;
            if (segment.fromNodeId !== lastNodeId) {
                lastGraphId = `${shipment.id}:${index}:start`;
                addStop(lastGraphId, segment.fromNodeId);
            }
            const target = `${shipment.id}:${index}:end`;
            addStop(target, segment.toNodeId);
            const blocked = isBlockedSegment(segment, incidents);
            const completed = live?.status === 'ready' || live?.status === 'issued' || (activeIndex > index) || (nextIndex > index);
            const active = !blocked && segment.id === live?.activeSegmentId;
            const color = blocked ? '#a82c42' : completed ? '#278165' : colors[variant];
            const label = `${formatMoney(segment.estimatedCostMinor)} · ${segmentDistance(segment)} · ${segmentDuration(segment)} мин`;
            edges.push({ id: `${shipment.id}:${segment.id}:${index}`, source: lastGraphId, target, type: 'routeLeg', animated: active,
                label, style: { stroke: color, strokeWidth: active ? 3 : 2, strokeDasharray: blocked || variant === 'hub' ? '7 5' : undefined },
                markerEnd: { type: MarkerType.ArrowClosed, color, width: 17, height: 17 } });
            lastNodeId = segment.toNodeId;
            lastGraphId = target;
        });

        if (variant !== 'return') {
            const id = `${shipment.id}:buyer`;
            addNode(id, 'Покупатель', 'Получение заказа');
            edges.push({ id: `${id}:link`, source: lastGraphId, target: id, type: 'straight', style: { strokeDasharray: '4 5' }, label: 'Выдача' });
        }

        const columns = laneNodes.length > 4 ? 3 : 2;
        const positioned = laneNodes.map((node, index) => {
            const row = Math.floor(index / columns);
            const column = row % 2 === 0 ? index % columns : columns - 1 - index % columns;
            return { ...node, position: { x: column * 310, y: laneTop + row * 175 } };
        });

        positioned.forEach((node, index) => nodes.push({
            ...node,
            sourcePosition: facing(node.position, positioned[index + 1]?.position ?? node.position, Position.Right),
            targetPosition: facing(node.position, positioned[index - 1]?.position ?? node.position, Position.Left),
        }));
        laneTop += Math.ceil(laneNodes.length / columns) * 175 + 45;
    });

    return { nodes, edges };
}
