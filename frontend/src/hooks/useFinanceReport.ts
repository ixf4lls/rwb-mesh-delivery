import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import type { FormEvent } from 'react';
import type { FinanceParty, FinancePeriod } from '../api/analyticsApi';
import { deliveryApi } from '../api/deliveryApi';
import { pvzApi } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import { defaultSellerPeriod, toApiPeriod } from '../functions/sellerPeriod';

type Report = { partyType: FinanceParty; partyId?: string; period: FinancePeriod };

export function useFinanceReport(demoRunId?: string, clock?: string) {
    const [partyType, selectParty] = useState<FinanceParty>('pvz');
    const [report, setReport] = useState<Report>();
    const [validation, setValidation] = useState('');
    const [dates] = useState(() => defaultSellerPeriod(clock));
    const points = pvzApi.useListPickupPointsInfiniteQuery(partyType === 'pvz' ? { demoRunId } : skipToken);
    const nodes = points.currentData?.pages.flatMap(({ items }) => items.map(({ node }) => node)) ?? [];
    const point = deliveryApi.useGetDeliveryPointEconomicsQuery(report?.partyType === 'pvz' && report.partyId
        ? { pointId: report.partyId, demoRunId, ...report.period } : skipToken);
    const market = deliveryApi.useGetDeliveryMarketplaceEconomicsQuery(report?.partyType === 'marketplace' ? { demoRunId, ...report.period } : skipToken);
    const seller = sellerApi.useGetSellerEconomicsQuery(report?.partyType === 'seller' && report.partyId
        ? { sellerId: report.partyId, demoRunId, ...report.period } : skipToken);

    function apply(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const fields = new FormData(event.currentTarget);
        const start = String(fields.get('start') ?? '');
        const end = String(fields.get('end') ?? '');
        const period = toApiPeriod(start, end);
        const partyId = String(fields.get('partyId') ?? '').trim();
        if (!period) {
            setValidation('Конец периода должен быть не раньше начала.');
            return;
        }
        if (partyType !== 'marketplace' && !/^[A-Za-z0-9_-]{1,128}$/.test(partyId)) {
            setValidation('Укажите участника отчёта.');
            return;
        }

        setValidation('');
        setReport({ partyType, partyId: partyType === 'marketplace' ? undefined : partyId, period });
    }

    return { partyType, selectParty, report, validation, dates, points, nodes, point, market, seller, apply };
}
