import type { Shipment } from '../api/deliveryApi';
import type { IncidentCreate, ReturnCase, StorageCase } from '../api/exceptionApi';

export const returnStatuses = { awaiting_dropoff: 'Ожидает сдачи', at_return_point: 'В пункте возврата', in_transit: 'В пути',
    awaiting_inspection: 'Ожидает осмотра', closed: 'Завершён', canceled: 'Отменён' };

export const storageStatuses = { open: 'Открыт', deferred: 'Отложен', transfer_planned: 'Вывоз запланирован', in_transit: 'В пути', closed: 'Закрыт' };

export const incidentStatuses = { open: 'Открыт', blocked: 'Маршрут заблокирован', resolved: 'Решён' };

export function physicalSegments(item: ReturnCase | StorageCase) {
    return item.route?.options.find(({ id }) => id === item.selectedOptionId)?.shipments.flatMap(({ segments }) => segments)
        .filter(({ mode }) => mode === 'physical') ?? [];
}

export function modelTimeFromInput(value: string, after: string) {
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return undefined;

    const time = Date.parse(`${value}:00Z`);
    return Number.isFinite(time) && time > Date.parse(after) ? new Date(time).toISOString() : undefined;
}

export function incidentTargets(shipment: Shipment | undefined) {
    const targets = new Map<string, Pick<IncidentCreate, 'type' | 'failureTarget'> & { label: string }>();
    for (const segment of shipment?.segments ?? []) {
        targets.set(`segment:${segment.id}`, { type: 'segment_failed', failureTarget: { segmentId: segment.id }, label: `Сбой на сегменте ${segment.id}` });
        if (segment.tripId) targets.set(`trip:${segment.tripId}`, { type: 'trip_disrupted', failureTarget: { tripId: segment.tripId }, label: `Сорван рейс ${segment.tripId}` });
        for (const nodeId of [segment.fromNodeId, segment.toNodeId]) {
            targets.set(`node:${nodeId}`, { type: 'node_unavailable', failureTarget: { nodeId }, label: `Недоступен узел ${nodeId}` });
        }
    }
    return targets;
}
