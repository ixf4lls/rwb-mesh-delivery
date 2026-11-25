import { analyticsApi } from '../api/analyticsApi';
import type { LedgerFilters } from '../api/analyticsApi';
import { useCursorPage } from './useCursorPage';

export function useFinanceLedger(filters: LedgerFilters) {
    const page = useCursorPage();
    const ledger = analyticsApi.useListFinanceLedgerQuery({ ...filters, limit: 20, cursor: page.cursor });

    return { page, ledger };
}
