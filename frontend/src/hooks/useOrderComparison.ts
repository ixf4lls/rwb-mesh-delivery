import { skipToken } from '@reduxjs/toolkit/query';
import { useId, useState } from 'react';
import type { FormEvent } from 'react';
import type { FinancePeriod } from '../api/analyticsApi';
import { deliveryApi } from '../api/deliveryApi';
import { pvzApi } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import { canCompareOrders } from '../functions/financeEntries';
import { defaultSellerPeriod, toApiPeriod } from '../functions/sellerPeriod';
import { useCursorPage } from './useCursorPage';

type Selection = { meshId: string; hubId: string; period: FinancePeriod };

export function useOrderComparison(demoRunId?: string, clock?: string) {
    const [selection, setSelection] = useState<Selection>();
    const [validation, setValidation] = useState('');
    const [party, setParty] = useState<'pvz' | 'marketplace'>('pvz');
    const [selectedSeller, selectSeller] = useState('');
    const [dates] = useState(() => defaultSellerPeriod(clock));
    const orderOptionsId = useId();
    const page = useCursorPage();
    const choices = pvzApi.useListOrdersQuery({ demoRunId, cursor: page.cursor });
    const mesh = deliveryApi.useGetDeliveryOrderQuery(selection ? { demoRunId, orderId: selection.meshId } : skipToken);
    const hub = deliveryApi.useGetDeliveryOrderQuery(selection ? { demoRunId, orderId: selection.hubId } : skipToken);
    const comparable = Boolean(mesh.currentData && hub.currentData && canCompareOrders(mesh.currentData, hub.currentData));
    const sellers = comparable ? [...new Set(mesh.currentData?.lines.map(({ sellerId }) => sellerId))] : [];
    const sellerId = sellers.includes(selectedSeller) ? selectedSeller : sellers[0];
    const meshFinance = sellerApi.useGetSellerOrderEconomicsQuery(selection && sellerId ? { demoRunId, sellerId, orderId: selection.meshId } : skipToken);
    const hubFinance = sellerApi.useGetSellerOrderEconomicsQuery(selection && sellerId ? { demoRunId, sellerId, orderId: selection.hubId } : skipToken);

    function compare(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const fields = new FormData(event.currentTarget);
        const meshId = String(fields.get('meshId') ?? '').trim();
        const hubId = String(fields.get('hubId') ?? '').trim();
        const period = toApiPeriod(String(fields.get('start') ?? ''), String(fields.get('end') ?? ''));
        if (!period || meshId === hubId || ![meshId, hubId].every((id) => /^[A-Za-z0-9_-]{1,128}$/.test(id))) {
            setValidation('Укажите два разных заказа и корректный период проводок.');
            return;
        }

        setValidation('');
        setSelection({ meshId, hubId, period });
    }

    return { selection, validation, party, setParty, sellers, sellerId, selectSeller, dates, orderOptionsId, choices, page, mesh, hub, comparable,
        meshFinance, hubFinance, compare };
}
