import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import type { FormEvent } from 'react';
import { sellerApi } from '../api/sellerApi';
import { defaultSellerPeriod, toApiPeriod } from '../functions/sellerPeriod';

export function useSellerEconomics(sellerId: string, demoRunId?: string, clock?: string) {
    const [dates, setDates] = useState(() => defaultSellerPeriod(clock));
    const [error, setError] = useState('');
    const period = toApiPeriod(dates.start, dates.end);
    const economics = sellerApi.useGetSellerEconomicsQuery(period ? { sellerId, demoRunId, ...period } : skipToken);

    function applyPeriod(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const fields = new FormData(event.currentTarget);
        const start = String(fields.get('start') ?? '');
        const end = String(fields.get('end') ?? '');

        if (!toApiPeriod(start, end)) {
            setError('Конец периода должен быть не раньше начала.');
            return;
        }

        setError('');
        setDates({ start, end });
    }

    return { dates, economics, error, applyPeriod };
}
