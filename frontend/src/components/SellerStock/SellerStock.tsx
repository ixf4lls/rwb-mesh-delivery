import { formatDate } from '../../functions/formatDate';
import { formatMoney } from '../../functions/money';
import { useSellerStock } from '../../hooks/useSellerStock';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './SellerStock.module.css';

const columns = ['Партия', 'Товар', 'Место хранения', 'Доступно', 'Резерв', 'Собрано', 'Карантин', 'Хранение'];

export function SellerStock({ sellerId, demoRunId }: { sellerId: string; demoRunId?: string }) {
    const view = useSellerStock(sellerId, demoRunId);
    const cost = view.storage.currentData;
    const rows = view.items.map((batch) => ({
        id: batch.id,
        cells: [<div><strong>{batch.id}</strong><small>{formatDate(batch.receivedAt)}</small></div>,
            view.productNames.get(batch.productId) ?? batch.productId, view.pointNames.get(batch.storagePointId) ?? batch.storagePointId,
            batch.availableQuantity, batch.reservedQuantity, batch.packedQuantity, batch.quarantinedQuantity,
            <div><small>{formatMoney(batch.storageRateMinorPerSlotDay)} / место в сутки</small>
                <Button variant="secondary" onClick={() => view.selectBatch(batch.id)} aria-haspopup="dialog">Начисления</Button></div>],
    }));

    return (
        <div className={styles.stock}>
            <p className={styles.hint}>Партии появятся здесь после приёмки на месте хранения.</p>
            <form className={styles.filter} onSubmit={view.filterPoints}>
                <FormField label="Место хранения">
                    <input name="pointId" list={view.pointListId} placeholder="Все места или ID ПВЗ / хаба" maxLength={128} pattern="[A-Za-z0-9_\-]+" />
                </FormField>
                <datalist id={view.pointListId}>
                    {view.pointIds.map((id) => <option key={id} value={id}>{view.pointNames.get(id) ?? id}</option>)}
                </datalist>
                <Button variant="secondary" type="submit">Применить</Button>
            </form>
            {view.batches.isError && <QueryNotice error={view.batches.error} onRetry={view.batches.refetch} />}
            {view.batches.isLoading && <QueryNotice message="Загружаем партии…" />}
            {view.batches.currentData && <div className={styles.table}>
                <DataTable title="Остатки по партиям" columns={columns} rows={rows} emptyMessage="Для выбранного места нет партий." />
                <Pagination cursor={view.page.cursor} nextCursor={view.batches.currentData.nextCursor} busy={view.batches.isFetching}
                    onPrevious={view.page.previous} onNext={view.page.next} />
            </div>}
            <Dialog title="Стоимость хранения" open={Boolean(view.batchId)} onClose={() => view.selectBatch('')}>
                <div className={styles.details}>
                    {view.storage.isError && <QueryNotice error={view.storage.error} onRetry={view.storage.refetch} />}
                    {view.batchId && !view.storage.isError && !cost && <QueryNotice message="Загружаем начисления…" />}
                    {cost && <dl>
                        <div><dt>Партия</dt><dd>{cost.batchId}</dd></div>
                        <div><dt>Тариф за место в сутки</dt><dd>{formatMoney(cost.rateMinorPerSlotDay)}</dd></div>
                        <div><dt>Версия тарифа</dt><dd>{cost.tariffVersionId}</dd></div>
                        <div><dt>Оплачиваемые место-сутки</dt><dd>{cost.billableSlotDays}</dd></div>
                        <div><dt>Начислено на {formatDate(cost.asOf)}</dt><dd>{formatMoney(cost.accruedMinor)}</dd></div>
                        <div><dt>Проведено</dt><dd>{formatMoney(cost.postedMinor)}</dd></div>
                    </dl>}
                    <Button variant="secondary" onClick={() => view.selectBatch('')}>Закрыть</Button>
                </div>
            </Dialog>
        </div>
    );
}
