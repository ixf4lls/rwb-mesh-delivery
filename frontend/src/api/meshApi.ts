import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';
import type { components, paths } from './schema.generated';

export type ApiContext = {
    getAccessToken?: () => string | undefined;
};

export type DemoScope = { demoRunId?: string };

export type AcceptedOperation = {
    operation: components['schemas']['Operation'];
    location: string | null;
    requestUrl?: string;
};

export function scopedHeaders(demoRunId?: string) {
    return demoRunId ? { 'X-Demo-Run-Id': demoRunId } : {};
}

type SessionResponse = paths['/me']['get']['responses'][200]['content']['application/json'];
type DemoContextResponse = paths['/demo/context']['get']['responses'][200]['content']['application/json'];
type OperationRequest = paths['/operations/{operationId}']['get'];
type OperationResponse = OperationRequest['responses'][200]['content']['application/json'];
type OperationArgs = OperationRequest['parameters']['path'] & NonNullable<OperationRequest['parameters']['header']>;

export const meshApi = createApi({
    reducerPath: 'meshApi',
    tagTypes: ['Capacity', 'Products', 'Batches', 'Orders', 'OrderRoute', 'DemoContext'],
    baseQuery: fetchBaseQuery({
        baseUrl: import.meta.env.VITE_API_BASE_URL || '/api/v1',
        timeout: 10000,
        isJsonContentType: (headers) => /\bapplication\/(?:[\w.-]+\+)?json\b/i.test(headers.get('content-type') ?? ''),
        prepareHeaders: (headers, { extra }) => {
            const context = extra as ApiContext | undefined;
            const token = context?.getAccessToken?.();

            headers.set('Accept', 'application/json, application/problem+json');
            if (token) {
                headers.set('Authorization', `Bearer ${token}`);
            }

            return headers;
        },
    }),
    endpoints: (builder) => ({
        getCurrentSession: builder.query<SessionResponse, void>({
            query: () => '/me',
        }),
        getDemoContext: builder.query<DemoContextResponse, void>({
            query: () => '/demo/context',
            providesTags: ['DemoContext'],
        }),
        getOperation: builder.query<OperationResponse, OperationArgs>({
            query: ({ operationId, 'X-Demo-Run-Id': demoRunId }) => ({
                url: `/operations/${encodeURIComponent(operationId)}`,
                headers: demoRunId ? { 'X-Demo-Run-Id': demoRunId } : undefined,
            }),
        }),
    }),
});
