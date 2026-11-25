import { pvzApi } from '../../api/pvzApi';
import type { Order } from '../../api/pvzApi';
import { formatDate } from '../../functions/formatDate';
import { getNextTrip, getOrderActions, orderStatusLabels } from '../../functions/pvzOrders';
import type { OrderAction } from '../../functions/pvzOrders';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './PvzOrderDetails.module.css';

type PvzOrderDetailsProps = {
    order: Order;
    pointId: string;
    demoRunId?: string;
    modelClock?: string;
    busy: boolean;
    onAction: (action: OrderAction) => void;
};

export function PvzOrderDetails({ order, pointId, demoRunId, modelClock, busy, onAction }: PvzOrderDetailsProps) {
    const route = pvzApi.useGetOrderRouteQuery({ orderId: order.id, revision: order.routeRevision, demoRunId });
    const plan = route.isError ? undefined : route.currentData;
    const actions = getOrderActions(order, pointId, plan);
    const nextTrip = plan && getNextTrip(order, plan, pointId, modelClock ? Date.parse(modelClock) : Date.now());
    const rows = order.lines.map((line) => ({ id: line.id, cells: [line.productId, line.sellerId, line.quantity] }));

    return (
        <div className={styles.panel}>
            <div className={styles.heading}>
                <span>Заказ</span>
                <h2>{order.id}</h2>
                <p>{orderStatusLabels[order.status]}</p>
            </div>
            <dl className={styles.facts}>
                <div><dt>Готовность к выдаче</dt><dd>{formatDate(order.estimatedReadyAt)}</dd></div>
                <div><dt>ПВЗ выдачи</dt><dd>{order.pickupPointId}</dd></div>
                {nextTrip && <div><dt>Ближайший рейс заказа</dt><dd>{nextTrip.tripId} · {formatDate(nextTrip.scheduledStartAt)}</dd></div>}
                {order.settlementStatus === 'pending' && <div><dt>Начисления</dt><dd>В обработке</dd></div>}
            </dl>
            {route.isError && <QueryNotice error={route.error} onRetry={route.refetch} />}
            <DataTable title="Состав заказа" columns={['Товар', 'Продавец', 'Количество']} rows={rows} emptyMessage="В заказе нет строк." />
            {actions.length > 0 && (
                <div className={styles.actions}>
                    {actions.map((action) => (
                        <Button key={`${action.body.type}:${'shipmentId' in action.body ? action.body.shipmentId : order.id}`} disabled={busy}
                            onClick={() => onAction(action)}>
                            {action.label}{'shipmentId' in action.body && order.shipments.length > 1 ? ` · ${action.body.shipmentId}` : ''}
                        </Button>
                    ))}
                </div>
            )}
        </div>
    );
}
