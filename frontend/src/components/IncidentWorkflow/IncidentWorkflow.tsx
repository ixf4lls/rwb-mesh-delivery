import type { Incident } from '../../api/deliveryApi';
import type { Order, RouteRevision } from '../../api/pvzApi';
import { formatHours } from '../../functions/deliveryRoutes';
import { formatDate } from '../../functions/formatDate';
import { incidentStatuses } from '../../functions/exceptionRoutes';
import { formatMoney } from '../../functions/money';
import { useIncidentWorkflow } from '../../hooks/useIncidentWorkflow';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './IncidentWorkflow.module.css';

type IncidentWorkflowProps = { order: Order; route?: RouteRevision; incidents: readonly Incident[]; demoRunId?: string };

export function IncidentWorkflow({ order, route, incidents, demoRunId }: IncidentWorkflowProps) {
    const view = useIncidentWorkflow(order, route, demoRunId);
    const rows = incidents.map((incident) => ({ id: incident.id, cells: [incident.id,
        incident.blockage?.detail ?? incident.description, incidentStatuses[incident.status],
        `${incident.oldRouteRevision} → ${incident.newRouteRevision ?? '—'}`, formatDate(incident.updatedAt),
        incident.status === 'resolved' ? '—' : <Button variant="secondary" disabled={view.routing.pending} onClick={() => void view.reroute(incident.id)}>
            {view.routing.pending ? 'Пересчитываем…' : 'Перестроить маршрут'}
        </Button>],
    }));

    return (
        <div className={styles.workflow}>
            {view.message && <QueryNotice message={view.message} />}
            {view.routing.error && <p className={styles.error} role="alert">{view.routing.error}</p>}
            {route?.previousRevision !== undefined && <div className={styles.delta}>
                <strong>Изменение после перестроения</strong>
                <p>Версия {route.previousRevision} → {route.revision}{route.deltaCostMinor === undefined ? ''
                    : ` · затраты ${route.deltaCostMinor > 0 ? '+' : ''}${formatMoney(route.deltaCostMinor)}`}
                {route.deltaEtaMinutes === undefined ? '' : ` · срок ${route.deltaEtaMinutes > 0 ? '+' : ''}${formatHours(route.deltaEtaMinutes)}`}</p>
            </div>}
            {view.shipments.length > 0 && <div><Button variant="secondary" aria-haspopup="dialog" onClick={view.openForm}>Зафиксировать сбой</Button></div>}
            {incidents.length > 0 && <DataTable title="Инциденты" columns={['Инцидент', 'Причина', 'Статус', 'Версия маршрута', 'Обновлён', 'Действие']}
                rows={rows} emptyMessage="Инцидентов нет." />}
            <Dialog title="Логистический сбой" open={view.open} onClose={view.close} busy={view.creation.pending}>
                {view.open && <form className={styles.form} onSubmit={(event) => void view.submit(event)}>
                    <p>Заказ <strong>{order.id}</strong></p>
                    <fieldset disabled={view.creation.pending}>
                        <FormField label="Отправление">
                            <select value={view.shipment?.id ?? ''} onChange={(event) => view.selectShipment(event.currentTarget.value)}>
                                {view.shipments.map(({ id }) => <option key={id} value={id}>{id}</option>)}
                            </select>
                        </FormField>
                        <FormField label="Проблемный ресурс">
                            <select key={view.shipment?.id} name="target" defaultValue="" required>
                                <option value="">Выберите сегмент, рейс или узел</option>
                                {[...view.targets].map(([key, value]) => <option key={key} value={key}>{value.label}</option>)}
                            </select>
                        </FormField>
                        <FormField label="Описание сбоя"><input name="description" maxLength={500} required /></FormField>
                    </fieldset>
                    {(view.validation || view.creation.error) && <p className={styles.error} role="alert">{view.validation || view.creation.error}</p>}
                    <div className={styles.actions}>
                        <Button type="submit" disabled={view.creation.pending}>{view.creation.pending ? 'Сохраняем…' : 'Зафиксировать'}</Button>
                        <Button variant="secondary" disabled={view.creation.pending} onClick={view.close}>Закрыть</Button>
                    </div>
                </form>}
            </Dialog>
        </div>
    );
}
