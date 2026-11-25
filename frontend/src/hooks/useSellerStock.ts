import { skipToken } from '@reduxjs/toolkit/query';
import { useId, useState } from 'react';
import type { FormEvent } from 'react';
import { pvzApi } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import { useCursorPage } from './useCursorPage';

export function useSellerStock(sellerId: string, demoRunId?: string) {
    const [pointId, setPointId] = useState('');
    const [batchId, selectBatch] = useState('');
    const pointListId = useId();
    const page = useCursorPage();
    const batches = pvzApi.useListBatchesQuery({ sellerId, pointId: pointId || undefined, demoRunId, cursor: page.cursor });
    const points = pvzApi.useListPickupPointsInfiniteQuery({ demoRunId });
    const products = pvzApi.useListProductsInfiniteQuery({ sellerId, demoRunId });
    const storage = sellerApi.useGetStorageCostQuery(batchId ? { batchId, demoRunId } : skipToken);
    const items = batches.currentData?.items ?? [];
    const pointNames = new Map(points.currentData?.pages.flatMap(({ items: nodes }) => nodes.map(({ node }) => [node.id, node.name] as const)));
    const productNames = new Map(products.currentData?.pages.flatMap(({ items: catalog }) => catalog.map(({ id, name }) => [id, name] as const)));
    const pointIds = [...new Set([...items.map(({ storagePointId }) => storagePointId), ...(pointId ? [pointId] : [])])];

    function filterPoints(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const nextPointId = String(new FormData(event.currentTarget).get('pointId') ?? '').trim();
        if (nextPointId && !/^[A-Za-z0-9_-]{1,128}$/.test(nextPointId)) return;

        page.reset();
        setPointId(nextPointId);
    }

    return { pointId, batchId, selectBatch, pointIds, pointNames, productNames, pointListId, filterPoints, storage, batches, items, page };
}
