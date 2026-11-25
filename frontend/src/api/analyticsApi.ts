import { meshApi, scopedHeaders } from './meshApi';
import type { DemoScope } from './meshApi';
import type { components, operations } from './schema.generated';

export type LedgerEntry = components['schemas']['LedgerEntry'];
export type FinancePeriod = components['schemas']['Period'];
export type FinanceParty = LedgerEntry['partyType'];
export type LedgerFilters = Omit<NonNullable<operations['listLedgerEntries']['parameters']['query']>, 'cursor'> & DemoScope;

export const analyticsApi = meshApi.injectEndpoints({
    endpoints: (builder) => ({
        listFinanceLedger: builder.query<components['schemas']['LedgerPage'], LedgerFilters & { cursor?: string }>({
            query: ({ demoRunId, ...params }) => ({ url: '/ledger-entries', params, headers: scopedHeaders(demoRunId) }),
            providesTags: ['Economics'],
        }),
    }),
});
