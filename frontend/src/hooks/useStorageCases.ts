import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import { deliveryApi } from '../api/deliveryApi';

export function useStorageCases(pointId: string, demoRunId?: string) {
    const [open, setOpen] = useState(false);
    const storage = deliveryApi.useListModelStorageInfiniteQuery(open ? { pointId: pointId || undefined, demoRunId } : skipToken,
        { pollingInterval: 15000 });

    return { setOpen, storage };
}
