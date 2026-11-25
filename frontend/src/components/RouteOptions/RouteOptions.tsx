import type { Quote } from '../../api/deliveryApi';
import { formatDate } from '../../functions/formatDate';
import { formatHours, policyNames } from '../../functions/deliveryRoutes';
import { formatMoney } from '../../functions/money';
import { Button } from '../Button/Button';
import styles from './RouteOptions.module.css';

type RouteOptionsProps = {
    quote?: Quote;
    selectedId: string;
    busy: boolean;
    canReserve: boolean;
    error: string;
    onSelect: (id: string) => void;
    onReserve: () => void;
};

export function RouteOptions({ quote, selectedId, busy, canReserve, error, onSelect, onReserve }: RouteOptionsProps) {
    return (
        <div className={styles.panel}>
            <h2>Варианты доставки</h2>
            {!quote ? <p>Варианты появятся после расчёта маршрутов для корзины.</p> : <>
                {quote.options.map((option) => <div key={option.id} className={styles.option} data-selected={selectedId === option.id}>
                    <h3>{policyNames[option.policy]}</h3>
                    {quote.recommendedOptionId === option.id && <span className={styles.recommended}>Рекомендация сервера</span>}
                    {option.feasible ? <dl>
                        <div><dt>Затраты</dt><dd>{option.estimatedCostMinor === undefined ? '—' : formatMoney(option.estimatedCostMinor)}</dd></div>
                        <div><dt>Для покупателя</dt><dd>{option.buyerFeeMinor === undefined ? '—' : formatMoney(option.buyerFeeMinor)}</dd></div>
                        <div><dt>Срок</dt><dd>{option.etaMinutes === undefined ? '—' : formatHours(option.etaMinutes)}</dd></div>
                    </dl> : <p className={styles.unavailable}>Маршрут недоступен</p>}
                    <ul>{option.reasons.map(({ message }, index) => <li key={index}>{message}</li>)}</ul>
                    {option.feasible && <Button variant="secondary" disabled={busy} aria-pressed={selectedId === option.id}
                        onClick={() => onSelect(option.id)}>Показать маршрут</Button>}
                </div>)}
                <small>Котировка {quote.id} действует до {formatDate(quote.expiresAt)}.</small>
                {canReserve && <Button disabled={busy} onClick={onReserve}>{busy ? 'Ожидаем резерв…' : error ? 'Повторить резервирование' : 'Создать заказ и резерв'}</Button>}
                {error && <p className={styles.unavailable} role="alert">{error}</p>}
            </>}
        </div>
    );
}
