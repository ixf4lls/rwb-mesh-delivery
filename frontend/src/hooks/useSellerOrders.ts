import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import { pvzApi } from '../api/pvzApi';
import type { Order } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import { useCursorPage } from './useCursorPage';

export function useSellerOrders(sellerId: string, demoRunId?: string) {
    const [status, setStatus] = useState<Order['status'] | ''>('');
    const [orderId, selectOrder] = useState('');
    const page = useCursorPage();
    const orders = pvzApi.useListOrdersQuery({ sellerId, demoRunId, status: status || undefined, cursor: page.cursor });
    const orderEconomics = sellerApi.useGetSellerOrderEconomicsQuery(orderId ? { sellerId, orderId, demoRunId } : skipToken);

    function filterStatus(value: Order['status'] | '') {
        page.reset();
        setStatus(value);
    }

    return { page, status, filterStatus, orderId, selectOrder, orders, orderEconomics };
}
