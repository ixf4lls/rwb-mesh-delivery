import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import { pvzApi } from '../api/pvzApi';
import { useDemoScope } from './useDemoScope';

export function usePvzContext() {
    const [panel, selectPanel] = useState<'orders' | 'stock'>('orders');
    const [selectedPointId, selectPoint] = useState('');
    const { demo, demoRunId, scopeReady } = useDemoScope();
    const points = pvzApi.useListPickupPointsInfiniteQuery(scopeReady ? { demoRunId } : skipToken);
    const pointsById = new Map(points.currentData?.pages.flatMap((page) => page.items.map((point) => [point.node.id, point] as const)));
    const pointIds = [...pointsById.keys()];
    const pointId = pointsById.has(selectedPointId) ? selectedPointId : (pointIds[0] ?? '');

    return { panel, selectPanel, demo, scopeReady, demoRunId, points, pointsById, pointIds, pointId, selectPoint };
}
