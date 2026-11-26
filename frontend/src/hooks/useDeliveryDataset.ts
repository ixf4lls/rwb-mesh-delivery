import type { FormEvent } from 'react';
import { deliveryApi } from '../api/deliveryApi';
import type { DemoRun } from '../api/deliveryApi';
import { useRepeatableRequest } from './useRepeatableRequest';

export function useDeliveryDataset(run?: DemoRun) {
    const datasets = deliveryApi.useListDeliveryDatasetsInfiniteQuery();
    const items = datasets.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const [importDatasets, importing] = deliveryApi.useImportDeliveryDatasetsMutation();
    async function load() {
        try {
            await importDatasets().unwrap();
            await datasets.refetch();
        } catch { return; }
    }
    const [start] = deliveryApi.useStartDeliveryRunMutation();
    const action = useRepeatableRequest((datasetId: string, key) => start({ body: { datasetId, expectedContextVersion: run?.contextVersion ?? 0 }, key }).unwrap());

    function apply(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const id = new FormData(event.currentTarget).get('datasetId');
        if (typeof id === 'string' && items.some((item) => item.id === id && item.status === 'ready')) void action.submit(id);
    }

    return { items, datasets, action, apply, load, importing };
}


