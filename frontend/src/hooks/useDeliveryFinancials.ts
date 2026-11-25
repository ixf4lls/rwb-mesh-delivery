import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import { deliveryApi } from '../api/deliveryApi';
import type { Order } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import { defaultSellerPeriod, toApiPeriod } from '../functions/sellerPeriod';

export function useDeliveryFinancials(order: Order, demoRunId?: string, clock?: string) {
    const [open, setOpen] = useState(false);
    const [selectedSeller, selectSeller] = useState('');
    const sellers = [...new Set(order.lines.map(({ sellerId }) => sellerId))];
    const sellerId = sellers.includes(selectedSeller) ? selectedSeller : sellers[0];
    const dates = defaultSellerPeriod(clock);
    const period = toApiPeriod(dates.start, dates.end);
    const market = deliveryApi.useGetDeliveryMarketplaceEconomicsQuery(open && period ? { demoRunId, ...period } : skipToken);
    const point = deliveryApi.useGetDeliveryPointEconomicsQuery(open && period ? { demoRunId, pointId: order.pickupPointId, ...period } : skipToken);
    const seller = sellerApi.useGetSellerOrderEconomicsQuery(open && sellerId ? { sellerId, demoRunId, orderId: order.id } : skipToken);

    return { dates, market, point, sellers, sellerId, seller, selectSeller, setOpen };
}
