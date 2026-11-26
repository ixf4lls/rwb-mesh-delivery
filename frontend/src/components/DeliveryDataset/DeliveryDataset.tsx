import type { DemoRun } from '../../api/deliveryApi';
import { formatDate } from '../../functions/formatDate';
import { useDeliveryDataset } from '../../hooks/useDeliveryDataset';
import { Button } from '../Button/Button';
import { DemoClock } from '../DemoClock/DemoClock';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './DeliveryDataset.module.css';

export function DeliveryDataset({ run }: { run?: DemoRun }) {
    const { items, datasets, action, apply, load, importing } = useDeliveryDataset(run);

    return (
        <details className={styles.panel} open>
            <summary>Демонаборы · {items.find(({ id }) => id === run?.datasetId)?.name ?? run?.datasetId ?? 'Выберите набор'}</summary>
            {run && <p>Модельное время {formatDate(run.clock)}.</p>}
            <Button variant="secondary" onClick={() => void load()} disabled={importing.isLoading}>{importing.isLoading ? 'Загрузка…' : 'Подгрузить наборы из /datasets'}</Button>
            {importing.isError && <QueryNotice error={importing.error} onRetry={load} />}
            {importing.isSuccess && <p role="status">Наборы загружены. Выберите сценарий для замены активных данных новым прогоном.</p>}
            <form onSubmit={apply}>
                <FormField label="Набор данных">
                    <select name="datasetId" required defaultValue="" disabled={action.pending}>
                        <option value="">Выберите сценарий</option>
                        {items.map((item) => <option key={item.id} value={item.id} disabled={item.status !== 'ready'}>{item.id} — {item.name}</option>)}
                    </select>
                </FormField>
                <Button type="submit" disabled={action.pending || !items.some(({ status }) => status === 'ready')}>Начать новый прогон</Button>
                {datasets.hasNextPage && <Button variant="secondary" onClick={() => void datasets.fetchNextPage()} disabled={datasets.isFetching}>Ещё наборы</Button>}
            </form>
            {action.error && <p role="alert">{action.error}</p>}
            {datasets.isError && <QueryNotice error={datasets.error} onRetry={datasets.refetch} />}
            {run && <DemoClock run={run} />}
        </details>
    );
}



