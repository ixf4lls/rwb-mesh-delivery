import { configureStore } from '@reduxjs/toolkit';
import { meshApi } from '../api/meshApi';
import type { ApiContext } from '../api/meshApi';

export function createAppStore(apiContext: ApiContext = {}) {
    return configureStore({
        reducer: {
            [meshApi.reducerPath]: meshApi.reducer,
        },
        middleware: (getDefaultMiddleware) => getDefaultMiddleware({
            thunk: { extraArgument: apiContext },
        }).concat(meshApi.middleware),
    });
}
