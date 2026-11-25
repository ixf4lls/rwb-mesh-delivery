import { skipToken } from '@reduxjs/toolkit/query';
import { deliveryApi } from '../../api/deliveryApi';
import type { DeliveryModel } from '../../hooks/useDeliveryModel';
import { formatDate } from '../../functions/formatDate';
import { formatMoney } from '../../functions/money';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './DeliveryEvents.module.css';

const incidentStatus = { open: 'Открыт', blocked: 'Маршрут заблокирован', resolved: 'Решён' };
const returnStatus = { awaiting_dropoff: 'Ожидает сдачи', at_return_point: 'В пункте возврата', in_transit: 'В пути',
    awaiting_inspection: 'Ожидает осмотра', closed: 'Завершён', canceled: 'Отменён' };

export function DeliveryEvents({ model, demoRunId }: { model: DeliveryModel; demoRunId?: string }) {
    const pointId = model.nodeId || model.order?.pickupPointId;
    const storage = deliveryApi.useListModelStorageInfiniteQuery(pointId ? { pointId, demoRunId } : skipToken);
    const incidents = model.incidents.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const storageRows = storage.currentData?.pages.flatMap(({ items }) => items.map((item) => ({
        id: item.id, cells: [item.batchId, `${item.ageDays} дн.`, `${item.thresholdDays} дн.`, formatMoney(item.cost.accruedMinor),
            formatMoney(item.cost.postedMinor), item.reason],
    }))) ?? [];

    if (!model.order && !pointId) return null;

    return (
        <div className={styles.events}>
            {model.order && <details className={styles.box}>
                <summary>Возвраты и логистические сбои</summary>
                <div className={styles.content}>
                    <FormField label="Обратный маршрут">
                        <select value={model.returnId} onChange={(event) => model.selectReturn(event.currentTarget.value)}>
                            <option value="">Исходная доставка</option>
                            {model.returnList.map(({ id, status }) => <option key={id} value={id}>{id} · {returnStatus[status]}</option>)}
                        </select>
                    </FormField>
                    {model.returns.isError && <QueryNotice error={model.returns.error} onRetry={model.returns.refetch} />}
                    {model.returns.hasNextPage && <Button variant="secondary" disabled={model.returns.isFetching} onClick={() => void model.returns.fetchNextPage()}>Ещё возвраты</Button>}
                    {model.incidents.isError && <QueryNotice error={model.incidents.error} onRetry={model.incidents.refetch} />}
                    {incidents.length > 0 && <DataTable title="Инциденты" columns={['Инцидент', 'Причина', 'Статус', 'Версия маршрута', 'Обновлён']}
                        rows={incidents.map((item) => ({ id: item.id, cells: [item.id, item.blockage?.detail ?? item.description,
                            incidentStatus[item.status], `${item.oldRouteRevision} → ${item.newRouteRevision ?? '—'}`, formatDate(item.updatedAt)] }))}
                        emptyMessage="Инцидентов нет." />}
                    {model.incidents.hasNextPage && <Button variant="secondary" disabled={model.incidents.isFetching} onClick={() => void model.incidents.fetchNextPage()}>Ещё инциденты</Button>}
                </div>
            </details>}
            {pointId && <details className={styles.box}>
                <summary>Длительное хранение в выбранном узле</summary>
                <div className={styles.content}>
                    {storage.isError && <QueryNotice error={storage.error} onRetry={storage.refetch} />}
                    {storage.isLoading && <QueryNotice message="Загружаем данные хранения…" />}
                    {storage.currentData && <DataTable title="Случаи длительного хранения"
                        columns={['Партия', 'На хранении', 'Порог', 'Начислено', 'Проведено', 'Решение сервера']}
                        rows={storageRows} emptyMessage="Случаев длительного хранения нет." />}
                    {storage.hasNextPage && <Button variant="secondary" disabled={storage.isFetching} onClick={() => void storage.fetchNextPage()}>Ещё случаи</Button>}
                </div>
            </details>}
        </div>
    );
}
