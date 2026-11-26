import { usePvzContext } from '../../hooks/usePvzContext';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { PvzCapacity } from '../PvzCapacity/PvzCapacity';
import { PvzOrders } from '../PvzOrders/PvzOrders';
import { PvzStock } from '../PvzStock/PvzStock';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './PvzWorkspace.module.css';

export function PvzWorkspace() {
    const view = usePvzContext();
    const point = view.pointsById.get(view.pointId)?.node;
    const pointLabel = point?.name ?? view.pointId;
    const scopeKey = `${view.demoRunId ?? ''}:${view.pointId}`;

    return (
        <div className={styles.workspace}>
            {view.pointId && (
                <div className={styles.toolbar}>
                    <div className={styles.point}>
                        <FormField label="Пункт выдачи">
                            <select value={view.pointId} onChange={(event) => view.selectPoint(event.currentTarget.value)}>
                                {view.pointIds.map((id) => <option key={id} value={id}>{view.pointsById.get(id)?.node.name ?? id}</option>)}
                            </select>
                        </FormField>
                        {point?.address && <p>{point.address}</p>}
                    </div>
                    {view.points.hasNextPage && (
                        <Button variant="secondary" disabled={view.points.isFetchingNextPage} onClick={() => void view.points.fetchNextPage()}>
                            Загрузить ещё ПВЗ
                        </Button>
                    )}
                </div>
            )}
            {view.demo.isError && <QueryNotice error={view.demo.error} onRetry={view.demo.refetch} />}
            {view.demo.isLoading && <QueryNotice message="Загружаем контекст демонстрации…" />}
            {view.points.isError && <QueryNotice error={view.points.error} onRetry={view.points.refetch} />}
            {view.scopeReady && !view.pointId && (view.points.isLoading ? (
                <QueryNotice message="Загружаем пункты выдачи…" />
            ) : !view.points.isError && <QueryNotice message="Сервер пока не вернул доступные ПВЗ." />)}
            {view.scopeReady && view.pointId && (
                <>
                    <PvzCapacity pointId={view.pointId} demoRunId={view.demoRunId} />
                    <div className={styles.switcher} role="group" aria-label="Рабочие разделы ПВЗ">
                        <Button variant={view.panel === 'orders' ? 'primary' : 'secondary'} aria-pressed={view.panel === 'orders'}
                            onClick={() => view.selectPanel('orders')}>Очередь заказов</Button>
                        <Button variant={view.panel === 'stock' ? 'primary' : 'secondary'} aria-pressed={view.panel === 'stock'}
                            onClick={() => view.selectPanel('stock')}>Приёмка и остатки</Button>
                    </div>
                    {view.panel === 'orders' ? (
                        <PvzOrders key={scopeKey} pointId={view.pointId} demoRunId={view.demoRunId} modelClock={view.demo.currentData?.clock} />
                    ) : (
                        <PvzStock key={scopeKey} pointId={view.pointId} pointLabel={pointLabel} demoRunId={view.demoRunId}
                            canReceive={!point || (point.active && point.kind === 'storage_pvz')} />
                    )}
                </>
            )}
        </div>
    );
}
