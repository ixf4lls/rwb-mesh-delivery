import { returnStatuses } from '../../functions/exceptionRoutes';
import type { DeliveryModel } from '../../hooks/useDeliveryModel';
import { useStorageCases } from '../../hooks/useStorageCases';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { IncidentWorkflow } from '../IncidentWorkflow/IncidentWorkflow';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { ReturnWorkflow } from '../ReturnWorkflow/ReturnWorkflow';
import { StorageWorkflow } from '../StorageWorkflow/StorageWorkflow';
import styles from './DeliveryEvents.module.css';

export function DeliveryEvents({ model, demoRunId, clock }: { model: DeliveryModel; demoRunId?: string; clock?: string }) {
    const pointId = model.nodeId || model.order?.pickupPointId || model.effectivePointId;
    const { setOpen, storage } = useStorageCases(pointId, demoRunId);
    const incidents = model.incidents.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const cases = storage.currentData?.pages.flatMap(({ items }) => items) ?? [];

    return (
        <div className={styles.events}>
            {model.order && <details className={styles.box}>
                <summary>Возвраты и логистические сбои</summary>
                <div className={styles.content}>
                    <FormField label="Обратный маршрут">
                        <select value={model.returnId} onChange={(event) => model.selectReturn(event.currentTarget.value)}>
                            <option value="">Исходная доставка</option>
                            {model.returnId && !model.activeReturn && <option value={model.returnId}>{model.returnId}</option>}
                            {model.returnList.map(({ id, status }) => <option key={id} value={id}>{id} · {returnStatuses[status]}</option>)}
                        </select>
                    </FormField>
                    {model.returns.isError && <QueryNotice error={model.returns.error} onRetry={model.returns.refetch} />}
                    {model.returns.hasNextPage && <Button variant="secondary" disabled={model.returns.isFetching} onClick={() => void model.returns.fetchNextPage()}>Ещё возвраты</Button>}
                    <ReturnWorkflow key={`return:${model.order.id}`} order={model.order} selected={model.activeReturn} points={model.nodeMap}
                        intake={model.returnPoints} demoRunId={demoRunId} onSelect={model.selectReturn} />
                    {model.incidents.isError && <QueryNotice error={model.incidents.error} onRetry={model.incidents.refetch} />}
                    <IncidentWorkflow key={`incident:${model.order.id}`} order={model.order} route={model.route.currentData} incidents={incidents} demoRunId={demoRunId} />
                    {model.incidents.hasNextPage && <Button variant="secondary" disabled={model.incidents.isFetching} onClick={() => void model.incidents.fetchNextPage()}>Ещё инциденты</Button>}
                </div>
            </details>}
            <details className={styles.box} onToggle={(event) => setOpen(event.currentTarget.open)}>
                <summary>{pointId ? 'Длительное хранение в выбранном узле' : 'Длительное хранение'}</summary>
                <div className={styles.content}>
                    {storage.isError && <QueryNotice error={storage.error} onRetry={storage.refetch} />}
                    {storage.isLoading && <QueryNotice message="Загружаем данные хранения…" />}
                    {storage.currentData && <StorageWorkflow key={pointId} cases={cases} demoRunId={demoRunId} clock={clock} />}
                    {storage.hasNextPage && <Button variant="secondary" disabled={storage.isFetching} onClick={() => void storage.fetchNextPage()}>Ещё случаи</Button>}
                </div>
            </details>
        </div>
    );
}
