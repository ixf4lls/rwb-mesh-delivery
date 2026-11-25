import type { Order } from '../../api/pvzApi';
import { formatDate } from '../../functions/formatDate';
import { orderStatusLabels } from '../../functions/pvzOrders';
import { usePvzOrders } from '../../hooks/usePvzOrders';
import { Button } from '../Button/Button';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { Icon } from '../Icon/Icon';
import { Pagination } from '../Pagination/Pagination';
import { PvzOrderDetails } from '../PvzOrderDetails/PvzOrderDetails';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './PvzOrders.module.css';

type PvzOrdersProps = { pointId: string; demoRunId?: string; modelClock?: string };

export function PvzOrders({ pointId, demoRunId, modelClock }: PvzOrdersProps) {
    const view = usePvzOrders(pointId, demoRunId);

    return (
        <>
            {view.success && <QueryNotice message={view.success} />}
            <div className={styles.layout}>
                <div className={styles.queue}>
                    <h2>Очередь заказов</h2>
                    <FormField label="Статус заказа">
                        <select value={view.status} onChange={(event) => view.selectStatus(event.currentTarget.value as Order['status'] | '')}>
                            <option value="">Все статусы</option>
                            {Object.entries(orderStatusLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
                        </select>
                    </FormField>
                    {view.orders.isError && <QueryNotice error={view.orders.error} onRetry={view.orders.refetch} />}
                    {!view.orders.isError && !view.orders.currentData && <QueryNotice message="Загружаем заказы…" />}
                    {view.orders.currentData && view.items.length === 0 && <QueryNotice message="По этому фильтру нет заказов." />}
                    <ul className={styles.list}>
                        {view.items.map((order) => (
                            <li key={order.id}>
                                <button type="button" className={styles.order} aria-pressed={view.order?.id === order.id}
                                    onClick={() => view.selectOrder(order.id)}>
                                    <span className={styles.glyph}><Icon name="pvz" /></span>
                                    <span className={styles.summary}>
                                        <strong>{order.id}</strong>
                                        <span>{orderStatusLabels[order.status]}</span>
                                        <span>Ожидаемая готовность {formatDate(order.estimatedReadyAt)}</span>
                                    </span>
                                </button>
                            </li>
                        ))}
                    </ul>
                    <Pagination cursor={view.page.cursor} nextCursor={view.orders.currentData?.nextCursor}
                        busy={view.orders.isFetching} onPrevious={view.page.previous} onNext={view.page.next} />
                </div>
                {view.order && <PvzOrderDetails order={view.order} pointId={pointId} demoRunId={demoRunId}
                    modelClock={modelClock} busy={view.orders.isFetching || view.orders.isError} onAction={view.openAction} />}
            </div>
            <Dialog title={view.action?.label ?? 'Подтверждение операции'} open={Boolean(view.action)} busy={view.command.pending} onClose={view.closeAction}>
                {view.action && (
                    <div className={styles.confirm}>
                        <p>Заказ <strong>{view.action.orderId}</strong></p>
                        {'shipmentId' in view.action.body && <p>Отправление {view.action.body.shipmentId}</p>}
                        {view.command.error && <p className={styles.error} role="alert">{view.command.error}</p>}
                        <div className={styles.actions}>
                            <Button disabled={view.command.pending} onClick={() => void view.confirmAction()}>
                                {view.command.pending ? 'Ожидаем подтверждения…' : view.command.error ? 'Повторить' : 'Подтвердить'}
                            </Button>
                            <Button variant="secondary" disabled={view.command.pending} onClick={view.closeAction}>Закрыть</Button>
                        </div>
                    </div>
                )}
            </Dialog>
        </>
    );
}
