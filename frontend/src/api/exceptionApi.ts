import { meshApi, scopedHeaders } from './meshApi';
import type { AcceptedOperation, DemoScope } from './meshApi';
import type { components } from './schema.generated';

type Schemas = components['schemas'];
export type ReturnCase = Schemas['Return'];
export type ReturnCommand = Schemas['ReturnCommand'];
export type StorageCase = Schemas['StorageCase'];
export type IncidentCreate = Schemas['IncidentCreate'];

export type ExceptionCommand =
    | { kind: 'createReturn'; body: Schemas['ReturnCreate'] }
    | { kind: 'recordReturn'; id: string; body: ReturnCommand }
    | { kind: 'reroute'; id: string; body: Schemas['RerouteCommand'] }
    | { kind: 'decideStorage'; id: string; body: Schemas['StorageDecision'] }
    | { kind: 'transferStorage'; id: string; body: Schemas['StorageTransferCommand'] }
    | { kind: 'advanceClock'; body: Schemas['ClockAdvance'] };

export const commandRoots = {
    createReturn: '/returns', recordReturn: '/returns', reroute: '/incidents', decideStorage: '/storage-cases',
    transferStorage: '/storage-cases', advanceClock: '/demo/clock/advances',
} as const;

const commandTails = { recordReturn: 'events', reroute: 'reroutes', decideStorage: 'decisions', transferStorage: 'transfer-events' } as const;

export const exceptionApi = meshApi.injectEndpoints({
    endpoints: (builder) => ({
        runExceptionCommand: builder.mutation<AcceptedOperation, DemoScope & { command: ExceptionCommand; key: string }>({
            query: ({ command, key, demoRunId }) => ({
                url: commandRoots[command.kind] + ('id' in command ? `/${encodeURIComponent(command.id)}/${commandTails[command.kind]}` : ''),
                method: 'POST', body: command.body, headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            transformResponse: (operation: Schemas['Operation'], meta) => ({
                operation, location: meta?.response?.headers.get('Location') ?? null, requestUrl: meta?.request.url,
            }),
        }),
        registerIncident: builder.mutation<Schemas['Incident'], DemoScope & { body: IncidentCreate; key: string }>({
            query: ({ body, key, demoRunId }) => ({
                url: '/incidents', method: 'POST', body, headers: { ...scopedHeaders(demoRunId), 'Idempotency-Key': key },
            }),
            invalidatesTags: (incident) => incident ? ['Incidents', 'OrderRoute', 'Orders', 'Notices'] : [],
        }),
        listNotices: builder.infiniteQuery<Schemas['NoticePage'], DemoScope & { unread?: boolean }, string | undefined>({
            infiniteQueryOptions: { initialPageParam: undefined, getNextPageParam: (last) => last.nextCursor },
            query: ({ queryArg: { demoRunId, unread }, pageParam }) => ({
                url: '/notifications', params: { cursor: pageParam, unread }, headers: scopedHeaders(demoRunId),
            }),
            providesTags: ['Notices'],
        }),
        readNotice: builder.mutation<Schemas['Notice'], DemoScope & { notificationId: string }>({
            query: ({ notificationId, demoRunId }) => ({
                url: `/notifications/${encodeURIComponent(notificationId)}/read`, method: 'PUT', headers: scopedHeaders(demoRunId),
            }),
            invalidatesTags: (notice) => notice ? ['Notices'] : [],
        }),
    }),
});
