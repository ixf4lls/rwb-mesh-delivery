import type { FulfillmentCommand, Order, RouteRevision } from '../api/pvzApi';

export type OrderAction = { label: string; orderId: string; body: FulfillmentCommand };

export const orderStatusLabels = {
    reserved: 'В резерве',
    packing: 'На сборке',
    in_transit: 'В пути',
    ready: 'Готов к выдаче',
    issued: 'Выдан',
    canceled: 'Отменён',
} satisfies Record<Order['status'], string>;

export function getOrderActions(order: Order, pointId: string, route?: RouteRevision): OrderAction[] {
    if (order.status === 'issued' || order.status === 'canceled') return [];

    const actions: OrderAction[] = [];
    const expectedVersion = order.version;

    for (const shipment of order.shipments) {
        const shipmentId = shipment.id;
        const segments = route?.shipments.find(({ id }) => id === shipmentId)?.segments ?? [];
        const departure = segments.find(({ id }) => id === shipment.nextSegmentId);
        const arrival = segments.find(({ id }) => id === shipment.activeSegmentId);

        if (shipment.status === 'reserved' && shipment.fromNodeId === pointId) {
            actions.push({ label: 'Подтвердить сборку', orderId: order.id, body: { type: 'packed', shipmentId, expectedVersion } });
        }
        if (['packed', 'at_node'].includes(shipment.status) && shipment.lastKnownNodeId === pointId
            && departure?.mode === 'physical' && departure.fromNodeId === pointId) {
            actions.push({
                label: 'Подтвердить передачу', orderId: order.id,
                body: { type: 'handed_over', shipmentId, segmentId: departure.id, expectedVersion },
            });
        }
        if (shipment.status === 'in_transit' && arrival?.mode === 'physical' && arrival.toNodeId === pointId) {
            actions.push({
                label: 'Подтвердить прибытие', orderId: order.id,
                body: { type: 'arrived', shipmentId, segmentId: arrival.id, expectedVersion },
            });
        }
    }

    if (order.status === 'ready' && order.pickupPointId === pointId) {
        actions.push({ label: 'Подтвердить выдачу', orderId: order.id, body: { type: 'issued', expectedVersion } });
    }

    if (order.shipments.every((shipment) => ['reserved', 'packed'].includes(shipment.status))) {
        actions.push({ label: 'Отменить заказ', orderId: order.id, body: { type: 'canceled', expectedVersion, reason: '' } });
    }
    return actions;
}

export function getNextTrip(order: Order, route: RouteRevision, pointId: string, now: number) {
    const nextSegments = new Set(order.shipments.map(({ nextSegmentId }) => nextSegmentId));

    return route.shipments.flatMap(({ segments }) => segments)
        .filter((segment) => segment.tripId && nextSegments.has(segment.id) && segment.fromNodeId === pointId
            && Date.parse(segment.scheduledStartAt) >= now)
        .sort((first, second) => Date.parse(first.scheduledStartAt) - Date.parse(second.scheduledStartAt))[0];
}
