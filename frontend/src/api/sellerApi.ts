import { meshApi, scopedHeaders } from './meshApi';
import type { DemoScope } from './meshApi';
import type { components, operations } from './schema.generated';

type Schemas = components['schemas'];
type SellerScope = DemoScope & { sellerId: string };
type SellerPeriod = operations['getSellerEconomics']['parameters']['query'];

export type ProductCreate = Schemas['ProductCreate'];
export type SellerEconomics = Schemas['SellerEconomics'];

export const sellerApi = meshApi.injectEndpoints({
    endpoints: (builder) => ({
        createProduct: builder.mutation<Schemas['Product'], DemoScope & { body: ProductCreate; key: string }>({
            query: ({ body, key, demoRunId }) => ({
                url: '/products',
                method: 'POST',
                body,
                headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            invalidatesTags: (product) => product ? ['Products'] : [],
        }),
        getStorageCost: builder.query<Schemas['StorageCost'], DemoScope & { batchId: string }>({
            query: ({ batchId, demoRunId }) => ({
                url: `/batches/${encodeURIComponent(batchId)}/storage-cost`,
                headers: scopedHeaders(demoRunId),
            }),
            providesTags: ['Batches'],
        }),
        getSellerEconomics: builder.query<SellerEconomics, SellerScope & SellerPeriod>({
            query: ({ sellerId, demoRunId, ...params }) => ({
                url: `/sellers/${encodeURIComponent(sellerId)}/economics`,
                params,
                headers: scopedHeaders(demoRunId),
            }),
        }),
        getSellerOrderEconomics: builder.query<Schemas['SellerOrderEconomics'], SellerScope & { orderId: string }>({
            query: ({ orderId, sellerId, demoRunId }) => ({
                url: `/sellers/${encodeURIComponent(sellerId)}/orders/${encodeURIComponent(orderId)}/economics`,
                headers: scopedHeaders(demoRunId),
            }),
            providesTags: ['Orders'],
        }),
    }),
});
