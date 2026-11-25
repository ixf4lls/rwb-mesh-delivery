import { skipToken } from '@reduxjs/toolkit/query';
import { meshApi } from '../api/meshApi';

const demoMode = import.meta.env.VITE_API_DEMO_MODE === 'true';

export function useDemoScope() {
    const demo = meshApi.useGetDemoContextQuery(demoMode ? undefined : skipToken);
    const demoRunId = demo.currentData?.id;

    return { demo, demoRunId, scopeReady: !demoMode || Boolean(demoRunId) };
}
