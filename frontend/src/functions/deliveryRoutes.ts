import type { Incident, LogisticsNode, RouteOption, Segment } from '../api/deliveryApi';

export const policyNames = { mesh_only: 'Mesh-доставка', hub_only: 'Через хаб', hybrid: 'Смешанный вариант' };

export const nodeKindNames = { pickup_only: 'ПВЗ выдачи', storage_pvz: 'ПВЗ хранения', sorting_center: 'Сортировочный центр', hub: 'Хаб' };

export const segmentNames = { on_site: 'На месте', handling: 'Обработка', sorting: 'Сортировка', city_trip: 'Городской рейс',
    linehaul: 'Магистраль', last_mile: 'Последняя миля' };

export const costNames = { storage: 'Хранение', handling: 'Обработка', sorting: 'Сортировка', linehaul: 'Магистраль',
    city_trip: 'Городской рейс', last_mile: 'Последняя миля', extra_stop: 'Дополнительная остановка', other: 'Прочее' };

const number = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 });

export function formatHours(minutes: number) {
    return `${number.format(minutes / 60)} ч`;
}

export function segmentDuration(segment: Segment) {
    return Math.max(0, Math.round((Date.parse(segment.scheduledEndAt) - Date.parse(segment.scheduledStartAt)) / 60000));
}

export function segmentDistance(segment: Segment) {
    return `${number.format(segment.distanceMeters / 1000)} км`;
}

export function optionDifference(mesh: RouteOption | undefined, hub: RouteOption | undefined, key: 'estimatedCostMinor' | 'buyerFeeMinor' | 'etaMinutes') {
    const meshValue = mesh?.[key];
    const hubValue = hub?.[key];
    if (!mesh?.feasible || !hub?.feasible || meshValue === undefined || hubValue === undefined) return undefined;

    return { amount: meshValue - hubValue, percent: hubValue === 0 ? undefined : (meshValue - hubValue) / hubValue * 100 };
}

export function pointName(nodes: ReadonlyMap<string, LogisticsNode>, id: string) {
    return nodes.get(id)?.name ?? id;
}

export function isBlockedSegment(segment: Segment, incidents: readonly Incident[]) {
    return incidents.some(({ status, failureTarget }) => status !== 'resolved' && (
        failureTarget.segmentId === segment.id || (segment.tripId && failureTarget.tripId === segment.tripId)
        || failureTarget.nodeId === segment.fromNodeId || failureTarget.nodeId === segment.toNodeId
    ));
}
