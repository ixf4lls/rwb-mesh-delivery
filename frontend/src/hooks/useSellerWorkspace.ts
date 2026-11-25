import { skipToken } from '@reduxjs/toolkit/query';
import { useId, useState } from 'react';
import type { FormEvent } from 'react';
import { pvzApi } from '../api/pvzApi';
import { useDemoScope } from './useDemoScope';

export const sellerPanels = {
    catalog: 'Товары', stock: 'Хранение и резервы', orders: 'Заказы', economics: 'Финансы',
};

export function useSellerWorkspace() {
    const { demo, demoRunId, scopeReady } = useDemoScope();
    const [selectedSellerId, setSelectedSellerId] = useState('');
    const [panel, selectPanel] = useState<keyof typeof sellerPanels>('catalog');
    const profileListId = useId();
    const discovery = pvzApi.useListProductsInfiniteQuery(scopeReady ? { demoRunId } : skipToken);
    const sellerIds = [...new Set(discovery.currentData?.pages.flatMap(({ items }) => items.map(({ sellerId }) => sellerId)))];
    const sellerId = selectedSellerId || (sellerIds.length === 1 ? sellerIds[0] : '');

    function applySeller(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const value = String(new FormData(event.currentTarget).get('sellerId') ?? '').trim();
        if (/^[A-Za-z0-9_-]{1,128}$/.test(value)) setSelectedSellerId(value);
    }

    return { demo, demoRunId, scopeReady, discovery, sellerIds, sellerId, applySeller, panel, selectPanel, profileListId };
}
