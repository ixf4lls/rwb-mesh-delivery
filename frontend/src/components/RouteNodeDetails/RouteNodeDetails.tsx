import { skipToken } from '@reduxjs/toolkit/query';
import type { LogisticsNode } from '../../api/deliveryApi';
import { pvzApi } from '../../api/pvzApi';
import { nodeKindNames } from '../../functions/deliveryRoutes';
import { formatMoney } from '../../functions/money';
import { useCursorPage } from '../../hooks/useCursorPage';
import { DataTable } from '../DataTable/DataTable';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './RouteNodeDetails.module.css';

export function RouteNodeDetails({ nodeId, node, demoRunId }: { nodeId: string; node?: LogisticsNode; demoRunId?: string }) {
    const isPoint = node?.kind === 'storage_pvz' || node?.kind === 'pickup_only';
    const page = useCursorPage();
    const args = { pointId: nodeId, demoRunId };
    const capacity = pvzApi.useGetPointCapacityQuery(isPoint ? args : skipToken);
    const batches = pvzApi.useListBatchesQuery({ ...args, cursor: page.cursor });
    const data = capacity.currentData;
    const rows = batches.currentData?.items.map((batch) => ({ id: batch.id,
        cells: [batch.productId, batch.availableQuantity, batch.reservedQuantity, `${formatMoney(batch.storageRateMinorPerSlotDay)} / место в сутки`],
    })) ?? [];

    return (
        <div className={styles.details}>
            <h2>{node?.name ?? nodeId}</h2>
            {node ? <p>{nodeKindNames[node.kind]}{node.address ? ` · ${node.address}` : ''}{node.active ? '' : ' · Недоступен'}</p>
                : <p>Справочник ПВЗ не содержит сведений об этом узле.</p>}
            {data && <p>Занято {data.storageUsedSlots + data.ordinaryUsedSlots + data.pickupUsedSlots} из {data.totalSlots} мест. Свободно для хранения {data.availableStorageSlots}.</p>}
            {capacity.isError && <QueryNotice error={capacity.error} onRetry={capacity.refetch} />}
            {batches.isError && <QueryNotice error={batches.error} onRetry={batches.refetch} />}
            {batches.isFetching && !batches.currentData && <QueryNotice message="Загружаем остатки узла…" />}
            {batches.currentData && <div>
                <DataTable title="Остатки узла" columns={['Товар', 'Доступно', 'В резерве', 'Тариф хранения']} rows={rows} emptyMessage="Нет партий на хранении." />
                <Pagination cursor={page.cursor} nextCursor={batches.currentData.nextCursor} busy={batches.isFetching} onPrevious={page.previous} onNext={page.next} />
            </div>}
        </div>
    );
}
