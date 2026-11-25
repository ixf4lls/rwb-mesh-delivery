import type { Order } from '../../api/pvzApi';
import { formatDate } from '../../functions/formatDate';
import { orderStatusLabels } from '../../functions/pvzOrders';
import { useSellerOrders } from '../../hooks/useSellerOrders';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { SellerAmounts } from '../SellerAmounts/SellerAmounts';
import styles from './SellerOrders.module.css';

const settlementLabels = { pending: 'В обработке', posted: 'Проведён', not_due: 'Ещё не начат' };

export function SellerOrders({ sellerId, demoRunId }: { sellerId: string; demoRunId?: string }) {
    const view = useSellerOrders(sellerId, demoRunId);
    const economics = view.orderEconomics.currentData;
    const rows = view.orders.currentData?.items.map((order) => ({
        id: order.id,
        cells: [<strong>{order.id}</strong>, formatDate(order.createdAt), orderStatusLabels[order.status],
            settlementLabels[order.settlementStatus], <Button variant="secondary" aria-haspopup="dialog"
                onClick={() => view.selectOrder(order.id)}>Расчёт</Button>],
    })) ?? [];

    return (
        <div className={styles.orders}>
            <div className={styles.filter}>
                <FormField label="Статус заказа">
                    <select value={view.status} onChange={(event) => view.filterStatus(event.currentTarget.value as Order['status'] | '')}>
                        <option value="">Все статусы</option>
                        {Object.entries(orderStatusLabels).map(([code, label]) => <option key={code} value={code}>{label}</option>)}
                    </select>
                </FormField>
            </div>
            {view.orders.isError && <QueryNotice error={view.orders.error} onRetry={view.orders.refetch} />}
            {view.orders.isLoading && <QueryNotice message="Загружаем заказы…" />}
            {view.orders.currentData && <div>
                <DataTable title="Заказы продавца" columns={['Заказ', 'Создан', 'Статус', 'Расчёт', 'Действия']}
                    rows={rows} emptyMessage="По этому фильтру нет заказов." />
                <Pagination cursor={view.page.cursor} nextCursor={view.orders.currentData.nextCursor} busy={view.orders.isFetching}
                    onPrevious={view.page.previous} onNext={view.page.next} />
            </div>}
            <Dialog title="Экономика заказа" open={Boolean(view.orderId)} onClose={() => view.selectOrder('')}>
                <div className={styles.details}>
                    <p>Заказ <strong>{view.orderId}</strong></p>
                    {view.orderEconomics.isError && <QueryNotice error={view.orderEconomics.error} onRetry={view.orderEconomics.refetch} />}
                    {view.orderId && !view.orderEconomics.isError && !economics && <QueryNotice message="Загружаем расчёт…" />}
                    {economics && <>
                        <p>Расчёт {settlementLabels[economics.settlementStatus].toLocaleLowerCase('ru')}.</p>
                        <SellerAmounts data={economics} />
                    </>}
                    <div><Button variant="secondary" onClick={() => view.selectOrder('')}>Закрыть</Button></div>
                </div>
            </Dialog>
        </div>
    );
}
