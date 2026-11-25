import { formatDate } from '../../functions/formatDate';
import { usePvzStock } from '../../hooks/usePvzStock';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './PvzStock.module.css';

type PvzStockProps = { pointId: string; pointLabel: string; demoRunId?: string; canReceive: boolean };

const columns = ['Партия', 'Товар и характеристики', 'Продавец', 'Место и дата приёмки', 'Принято', 'Доступно', 'Резерв', 'Собрано', 'Карантин'];

export function PvzStock({ pointId, pointLabel, demoRunId, canReceive }: PvzStockProps) {
    const view = usePvzStock(pointId, demoRunId);
    const rows = (view.batches.currentData?.items ?? []).map((batch) => {
        const product = view.productsById.get(batch.productId);

        return {
            id: batch.id,
            cells: [
                <div><strong>{batch.id}</strong>{batch.externalLotRef && <small>{batch.externalLotRef}</small>}</div>,
                <div>
                    <strong>{product?.name ?? batch.productId}</strong>
                    {product && <small>{[product.sellerSku, product.category, `${product.weightGrams} г`, `${product.slotsPerUnit} мест/шт.`]
                        .filter(Boolean).join(' · ')}</small>}
                </div>,
                batch.sellerId,
                <div>{pointLabel}<small>{formatDate(batch.receivedAt)}</small></div>,
                batch.receivedQuantity, batch.availableQuantity, batch.reservedQuantity, batch.packedQuantity, batch.quarantinedQuantity,
            ],
        };
    });

    return (
        <div className={styles.stock}>
            <div className={styles.toolbar}>
                <Button onClick={view.openReceipt} disabled={!canReceive}>Принять партию</Button>
                {!canReceive && <p>В этом ПВЗ приёмка на хранение недоступна.</p>}
            </div>
            {view.success && <QueryNotice message={view.success} />}
            {view.products.isError && <QueryNotice error={view.products.error} onRetry={view.products.refetch} />}
            {view.batches.isError && <QueryNotice error={view.batches.error} onRetry={view.batches.refetch} />}
            {!view.batches.isError && !view.batches.currentData && <QueryNotice message="Загружаем остатки…" />}
            {view.batches.currentData && (
                <div className={styles.table}>
                    <DataTable title="Остатки по партиям" columns={columns} rows={rows} emptyMessage="В выбранном ПВЗ нет партий на хранении." />
                    <Pagination cursor={view.page.cursor} nextCursor={view.batches.currentData.nextCursor} busy={view.batches.isFetching}
                        onPrevious={view.page.previous} onNext={view.page.next} />
                </div>
            )}
            <Dialog title="Приёмка партии" open={view.receiving} busy={view.receipt.pending} onClose={view.closeReceipt}>
                <form className={styles.form} onSubmit={(event) => void view.submitReceipt(event)}>
                    <p>Место хранения: <strong>{pointLabel}</strong></p>
                    {view.products.isLoading && <QueryNotice message="Загружаем каталог…" />}
                    {view.products.isError && <QueryNotice error={view.products.error} onRetry={view.products.refetch} />}
                    <FormField label="Товар">
                        <select name="productId" defaultValue="" required disabled={view.receipt.pending || view.catalog.length === 0}>
                            <option value="" disabled>Выберите товар</option>
                            {view.catalog.map((product) => (<option key={product.id} value={product.id}>
                                {product.name} · {product.sellerSku} · {product.sellerId}
                            </option>))}
                        </select>
                    </FormField>
                    {view.products.hasNextPage && (
                        <Button variant="secondary" disabled={view.receipt.pending || view.products.isFetchingNextPage}
                            onClick={() => void view.products.fetchNextPage()}>Загрузить ещё товары</Button>
                    )}
                    {!view.products.isFetching && !view.products.isError && view.catalog.length === 0 && <p>В каталоге пока нет товаров.</p>}
                    <FormField label="Количество, шт.">
                        <input type="number" name="quantity" min={1} max={Number.MAX_SAFE_INTEGER} step={1} required disabled={view.receipt.pending} />
                    </FormField>
                    <FormField label="Внешний номер партии (необязательно)">
                        <input name="externalLotRef" type="text" maxLength={128} disabled={view.receipt.pending} />
                    </FormField>
                    {(view.validation || view.receipt.error) && <p className={styles.error} role="alert">{view.validation || view.receipt.error}</p>}
                    <div className={styles.toolbar}>
                        <Button type="submit" disabled={view.receipt.pending || view.catalog.length === 0 || !canReceive}>
                            {view.receipt.pending ? 'Принимаем партию…' : view.receipt.error ? 'Повторить' : 'Подтвердить приёмку'}
                        </Button>
                        <Button variant="secondary" disabled={view.receipt.pending} onClick={view.closeReceipt}>Закрыть</Button>
                    </div>
                </form>
            </Dialog>
        </div>
    );
}
