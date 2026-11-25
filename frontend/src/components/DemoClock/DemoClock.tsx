import type { DemoRun } from '../../api/deliveryApi';
import { useDemoClock } from '../../hooks/useDemoClock';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './DemoClock.module.css';

export function DemoClock({ run }: { run: DemoRun }) {
    const { advance, validation, advanced, command } = useDemoClock(run);

    return (
        <div className={styles.clock}>
            <h3>Модельные часы</h3>
            <form className={styles.form} onSubmit={(event) => void advance(event)}>
                <FormField label="Перевести время на, UTC"><input name="to" type="datetime-local" required disabled={command.pending} /></FormField>
                <Button type="submit" disabled={command.pending}>{command.pending ? 'Обрабатываем…' : 'Перевести часы'}</Button>
            </form>
            {(validation || command.error) && <p className={styles.error} role="alert">{validation || command.error}</p>}
            {advanced && <QueryNotice message="Сервер подтвердил новое время. Данные по хранению и уведомления обновлены." />}
        </div>
    );
}
