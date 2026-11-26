import { meshApi } from '../api/meshApi';



export function useDemoScope() {
    const demo = meshApi.useGetDemoContextQuery(undefined);
    const demoRunId = demo.currentData?.id;

    return { demo, demoRunId, scopeReady: Boolean(demoRunId) };
}

