import { meshApi, scopedHeaders } from './meshApi';
import type { DemoScope as Scope } from './meshApi';
import type { components, operations } from './schema.generated';

type Schemas = components['schemas'];
type ScopedQuery<Query> = Scope & NonNullable<Query>;

export type Capacity = Schemas['Capacity'];
export type Product = Schemas['Product'];
export type Batch = Schemas['Batch'];
export type BatchReceipt = Schemas['BatchReceipt'];
export type Order = Schemas['Order'];
export type FulfillmentCommand = Schemas['FulfillmentCommand'];
export type RouteRevision = Schemas['RouteRevision'];

export const pvzApi = meshApi.injectEndpoints({
    endpoints: (builder) => ({
        listPickupPoints: builder.infiniteQuery<Schemas['PickupPointPage'], Scope, string | undefined>({
            infiniteQueryOptions: {
                initialPageParam: undefined,
                getNextPageParam: (lastPage) => lastPage.nextCursor,
            },
            query: ({ queryArg: { demoRunId }, pageParam }) => ({
                url: '/pickup-points',
                headers: scopedHeaders(demoRunId),
                params: { limit: 100, cursor: pageParam },
            }),
        }),
        listProducts: builder.infiniteQuery<Schemas['ProductPage'], Scope & { sellerId?: string }, string | undefined>({
            infiniteQueryOptions: {
                initialPageParam: undefined,
                getNextPageParam: (lastPage) => lastPage.nextCursor,
            },
            query: ({ queryArg: { demoRunId, sellerId }, pageParam }) => ({
                url: '/products',
                headers: scopedHeaders(demoRunId),
                params: { limit: 100, cursor: pageParam, sellerId },
            }),
            providesTags: ['Products'],
        }),
        getPointCapacity: builder.query<Capacity, Scope & { pointId: string }>({
            query: ({ pointId, demoRunId }) => ({
                url: `/pickup-points/${encodeURIComponent(pointId)}/capacity`,
                headers: scopedHeaders(demoRunId),
            }),
            providesTags: ['Capacity'],
        }),
        listBatches: builder.query<Schemas['BatchPage'], ScopedQuery<operations['listBatches']['parameters']['query']>>({
            query: ({ demoRunId, ...params }) => ({ url: '/batches', params, headers: scopedHeaders(demoRunId) }),
            providesTags: ['Batches'],
        }),
        receiveBatch: builder.mutation<Batch, Scope & { body: BatchReceipt; key: string }>({
            query: ({ body, key, demoRunId }) => ({
                url: '/batches',
                method: 'POST',
                body,
                headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            invalidatesTags: (batch) => batch ? ['Batches', 'Capacity'] : [],
        }),
        listOrders: builder.query<Schemas['OrderPage'], ScopedQuery<operations['listOrders']['parameters']['query']>>({
            query: ({ demoRunId, ...params }) => ({ url: '/orders', params, headers: scopedHeaders(demoRunId) }),
            providesTags: ['Orders'],
        }),
        getOrderRoute: builder.query<RouteRevision, Scope & { orderId: string; revision: number }>({
            query: ({ orderId, demoRunId, revision }) => ({
                url: `/orders/${encodeURIComponent(orderId)}/route`,
                headers: scopedHeaders(demoRunId),
                params: { revision },
            }),
            providesTags: ['OrderRoute'],
        }),
        recordOrderEvent: builder.mutation<{
            operation: Schemas['Operation']; location: string | null; requestUrl: string | undefined;
        }, Scope & { orderId: string; body: FulfillmentCommand; key: string }>({
            query: ({ orderId, body, key, demoRunId }) => ({
                url: `/orders/${encodeURIComponent(orderId)}/events`,
                method: 'POST',
                body,
                headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            transformResponse: (operation: Schemas['Operation'], meta) => ({
                operation,
                location: meta?.response?.headers.get('Location') ?? null,
                requestUrl: meta?.request.url,
            }),
        }),
    }),
});
