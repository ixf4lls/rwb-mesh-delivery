import { meshApi, scopedHeaders } from './meshApi';
import type { AcceptedOperation, DemoScope } from './meshApi';
import type { components, operations } from './schema.generated';

type Schemas = components['schemas'];
type Command<T> = DemoScope & { body: T; key: string };
type OrderScope = DemoScope & { orderId: string };
type Query<T> = DemoScope & NonNullable<T>;

export type Quote = Schemas['Quote'];
export type QuoteRequest = Schemas['QuoteRequest'];
export type RouteOption = Schemas['RouteOption'];
export type Shipment = Schemas['PlannedShipment'];
export type Segment = Schemas['Segment'];
export type RouteReason = Schemas['RoutingReason'];
export type LogisticsNode = Schemas['Node'];
export type DemoRun = Schemas['DemoRun'];
export type Incident = Schemas['Incident'];

export const deliveryApi = meshApi.injectEndpoints({
    endpoints: (builder) => ({
        listCities: builder.infiniteQuery<Schemas['CityPage'], DemoScope, string | undefined>({
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ queryArg: { demoRunId }, pageParam }) => ({
                url: '/cities', headers: scopedHeaders(demoRunId), params: { limit: 100, cursor: pageParam },
            }),
        }),
        createQuote: builder.mutation<Quote, Command<QuoteRequest>>({
            query: ({ body, key, demoRunId }) => ({
                url: '/quotes', method: 'POST', body, headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
        }),
        createDeliveryOrder: builder.mutation<AcceptedOperation, Command<Schemas['OrderCreate']>>({
            query: ({ body, key, demoRunId }) => ({
                url: '/orders', method: 'POST', body, headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            transformResponse: (operation: Schemas['Operation'], meta) => ({
                operation, location: meta?.response?.headers.get('Location') ?? null, requestUrl: meta?.request.url,
            }),
        }),
        getDeliveryOrder: builder.query<Schemas['Order'], OrderScope>({
            query: ({ orderId, demoRunId }) => ({ url: `/orders/${encodeURIComponent(orderId)}`, headers: scopedHeaders(demoRunId) }),
            providesTags: ['Orders'],
        }),
        listModelReturns: builder.infiniteQuery<Schemas['ReturnPage'], OrderScope, string | undefined>({
            providesTags: ['Returns'],
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ queryArg: { demoRunId, orderId }, pageParam }) => ({
                url: '/returns', params: { orderId, cursor: pageParam }, headers: scopedHeaders(demoRunId),
            }),
        }),
        listModelIncidents: builder.infiniteQuery<Schemas['IncidentPage'], OrderScope, string | undefined>({
            providesTags: ['Incidents'],
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ queryArg: { demoRunId, orderId }, pageParam }) => ({
                url: '/incidents', params: { orderId, cursor: pageParam }, headers: scopedHeaders(demoRunId),
            }),
        }),
        listModelStorage: builder.infiniteQuery<Schemas['StorageCasePage'], Query<operations['listStorageCases']['parameters']['query']>, string | undefined>({
            providesTags: ['StorageCases'],
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ queryArg: { demoRunId, ...params }, pageParam }) => ({
                url: '/storage-cases', params: { ...params, cursor: pageParam }, headers: scopedHeaders(demoRunId),
            }),
        }),
        listDeliveryDatasets: builder.infiniteQuery<Schemas['DatasetPage'], void, string | undefined>({
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ pageParam }) => ({ url: '/demo/datasets', params: { cursor: pageParam } }),
        }),
        startDeliveryRun: builder.mutation<DemoRun, Command<Schemas['DemoRunCreate']>>({
            query: ({ body, key }) => ({ url: '/demo/runs', method: 'POST', body, headers: { 'Idempotency-Key': key } }),
            invalidatesTags: (run) => run ? ['DemoContext'] : [],
        }),
        getDeliveryPointEconomics: builder.query<Schemas['PointEconomics'], DemoScope & { pointId: string; from: string; to: string }>({
            providesTags: ['Economics'],
            query: ({ pointId, demoRunId, ...params }) => ({
                url: `/pickup-points/${encodeURIComponent(pointId)}/economics`, params, headers: scopedHeaders(demoRunId),
            }),
        }),
        getDeliveryMarketplaceEconomics: builder.query<Schemas['MarketplaceEconomics'], DemoScope & { from: string; to: string }>({
            providesTags: ['Economics'],
            query: ({ demoRunId, ...params }) => ({ url: '/marketplace/economics', params, headers: scopedHeaders(demoRunId) }),
        }),
    }),
});
