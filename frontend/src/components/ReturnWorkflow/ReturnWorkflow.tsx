import type { LogisticsNode } from '../../api/deliveryApi';
import type { ReturnCase } from '../../api/exceptionApi';
import type { Order } from '../../api/pvzApi';
import { pointName } from '../../functions/deliveryRoutes';
import { formatDate } from '../../functions/formatDate';
import { returnStatuses } from '../../functions/exceptionRoutes';
import { useReturnWorkflow } from '../../hooks/useReturnWorkflow';
import { Button } from '../Button/Button';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './ReturnWorkflow.module.css';

type ReturnWorkflowProps = {
    order: Order;
    selected?: ReturnCase;
    points: ReadonlyMap<string, LogisticsNode>;
    intake: readonly LogisticsNode[];
    demoRunId?: string;
    onSelect: (id: string) => void;
};

const titles = { create: 'Оформить возврат', received_from_customer: 'Принять возврат', handed_over: 'Подтвердить передачу',
    arrived: 'Подтвердить прибытие', inspected: 'Подтвердить осмотр', canceled: 'Отменить возврат' };

export function ReturnWorkflow({ order, selected, points, intake, demoRunId, onSelect }: ReturnWorkflowProps) {
    const view = useReturnWorkflow(order, selected, demoRunId, onSelect);
    const action = view.form;
    const moving = action === 'handed_over' || action === 'arrived';
    const lineLabels = new Map(order.lines.map(({ id, productId }) => [id, productId]));

    return (
        <div className={styles.workflow}>
            {view.message && <QueryNotice message={view.message} />}
            {selected && <div className={styles.summary}>
                <strong>{selected.id} · {returnStatuses[selected.status]}</strong>
                <span>Принять в {pointName(points, selected.returnPointId)} · осмотр в {pointName(points, selected.inspectionNodeId)}</span>
                <span>Срок сдачи {formatDate(selected.dropoffBy)}. {selected.settlementStatus === 'posted' ? 'Начисления проведены.'
                    : selected.settlementStatus === 'pending' ? 'Начисления обрабатываются.' : ''}</span>
            </div>}
            <div className={styles.actions}>
                {order.status === 'issued' && <Button variant="secondary" aria-haspopup="dialog" onClick={() => view.open('create')}>{titles.create}</Button>}
                {selected?.status === 'awaiting_dropoff' && <>
                    <Button aria-haspopup="dialog" onClick={() => view.open('received_from_customer')}>{titles.received_from_customer}</Button>
                    <Button variant="secondary" aria-haspopup="dialog" onClick={() => view.open('canceled')}>{titles.canceled}</Button>
                </>}
                {selected?.status === 'at_return_point' && view.segments.length > 0 && <Button aria-haspopup="dialog" onClick={() => view.open('handed_over')}>{titles.handed_over}</Button>}
                {selected?.status === 'in_transit' && view.segments.length > 0 && <Button aria-haspopup="dialog" onClick={() => view.open('arrived')}>{titles.arrived}</Button>}
                {selected?.status === 'awaiting_inspection' && <Button aria-haspopup="dialog" onClick={() => view.open('inspected')}>{titles.inspected}</Button>}
            </div>
            <Dialog title={action ? titles[action] : 'Возврат'} open={Boolean(action)} onClose={view.close} busy={view.command.pending}>
                {action && <form className={styles.form} onSubmit={(event) => void view.submit(event)}>
                    <p>Заказ <strong>{order.id}</strong>{action !== 'create' && selected && ` · возврат ${selected.id}`}</p>
                    <fieldset disabled={view.command.pending}>
                        {action === 'create' && <>
                            <FormField label="ПВЗ приёма возврата">
                                <select name="returnPointId" required defaultValue={intake.some(({ id }) => id === order.pickupPointId) ? order.pickupPointId : ''}>
                                    <option value="">Выберите ПВЗ</option>
                                    {intake.map(({ id, name }) => <option key={id} value={id}>{name}</option>)}
                                </select>
                            </FormField>
                            {order.lines.map((line) => <FormField key={line.id} label={`${line.productId} · выдано ${line.quantity} шт.`}>
                                <input name={`quantity:${line.id}`} type="number" min={0} max={line.quantity} step={1} defaultValue={0} required />
                            </FormField>)}
                        </>}
                        {(action === 'create' || action === 'canceled') && <FormField label="Причина">
                            <input name="reason" maxLength={500} required />
                        </FormField>}
                        {moving && <FormField label="Сегмент обратного маршрута">
                            <select name="segmentId" defaultValue={view.segments.length === 1 ? view.segments[0].id : ''} required>
                                <option value="">Выберите сегмент</option>
                                {view.segments.map((segment) => <option key={segment.id} value={segment.id}>
                                    {pointName(points, segment.fromNodeId)} → {pointName(points, segment.toNodeId)}
                                </option>)}
                            </select>
                        </FormField>}
                        {action === 'inspected' && <>
                            <p>Укажите годные единицы. Остальные единицы попадут в карантин.</p>
                            {selected?.lines.map((line) => <FormField key={line.orderItemId} label={`${lineLabels.get(line.orderItemId) ?? line.orderItemId} · годных из ${line.quantity}`}>
                                <input name={`good:${line.orderItemId}`} type="number" min={0} max={line.quantity} step={1} defaultValue={line.quantity} required />
                            </FormField>)}
                        </>}
                    </fieldset>
                    {(view.validation || view.command.error) && <p className={styles.error} role="alert">{view.validation || view.command.error}</p>}
                    <div className={styles.actions}>
                        <Button type="submit" disabled={view.command.pending}>{view.command.pending ? 'Ожидаем подтверждения…' : 'Подтвердить'}</Button>
                        <Button variant="secondary" disabled={view.command.pending} onClick={view.close}>Закрыть</Button>
                    </div>
                </form>}
            </Dialog>
        </div>
    );
}
