import type { DeliveryModel } from '../../hooks/useDeliveryModel';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './DeliveryControls.module.css';

export function DeliveryControls({ model }: { model: DeliveryModel }) {
    const cities = model.cities.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const citiesById = new Map(cities.map(({ id, name }) => [id, name]));
    const orders = model.orders.currentData?.items ?? [];
    const busy = model.quoteCommand.pending || model.reservation.pending;
    const externalOrder = model.orderId && !orders.some(({ id }) => id === model.orderId);

    return (
        <div className={styles.panel}>
            <h2>Параметры доставки</h2>
            <form className={styles.form} onSubmit={(event) => void model.calculate(event)}>
                <fieldset disabled={busy}>
                    <FormField label="Город получения">
                        <select value={model.cityId} onChange={(event) => model.selectCity(event.currentTarget.value)}>
                            <option value="">Все города</option>
                            {cities.map(({ id, name }) => <option key={id} value={id}>{name}</option>)}
                        </select>
                    </FormField>
                    {model.cities.hasNextPage && <Button variant="secondary" onClick={() => void model.cities.fetchNextPage()} disabled={model.cities.isFetching}>Ещё города</Button>}
                    <FormField label="ПВЗ получения">
                        <select required value={model.effectivePointId} onChange={(event) => model.selectPoint(event.currentTarget.value)}>
                            <option value="">Выберите ПВЗ</option>
                            {model.destinationPoints.map(({ node }) => <option key={node.id} value={node.id}>
                                {model.cityId ? node.name : [citiesById.get(node.cityId), node.name].filter(Boolean).join(' · ')}
                            </option>)}
                        </select>
                    </FormField>
                    {model.points.hasNextPage && <Button variant="secondary" onClick={() => void model.points.fetchNextPage()} disabled={model.points.isFetching}>Ещё ПВЗ</Button>}
                    <FormField label="Товар">
                        <select value={model.productId} onChange={(event) => model.selectProduct(event.currentTarget.value)} required>
                            <option value="">Выберите товар</option>
                            {model.productList.map(({ id, name, sellerSku }) => <option key={id} value={id}>{name} · {sellerSku}</option>)}
                        </select>
                    </FormField>
                    {model.products.hasNextPage && <Button variant="secondary" onClick={() => void model.products.fetchNextPage()} disabled={model.products.isFetching}>Ещё товары</Button>}
                    <FormField label="Количество, шт."><input type="number" min={1} step={1} value={model.quantity}
                        onChange={(event) => model.setQuantity(event.currentTarget.value)} required /></FormField>
                    <Button type="submit" disabled={!model.effectivePointId || model.productList.length === 0}>
                        {model.quoteCommand.pending ? 'Рассчитываем…' : 'Сравнить маршруты'}
                    </Button>
                </fieldset>
                {(model.inputError || model.quoteCommand.error) && <p className={styles.error} role="alert">{model.inputError || model.quoteCommand.error}</p>}
                {model.quoteNeedsRefresh && <p className={styles.error}>Параметры изменились. Рассчитайте новую котировку перед резервированием.</p>}
            </form>
            {model.cities.isError && <QueryNotice error={model.cities.error} onRetry={model.cities.refetch} />}
            {model.points.isError && <QueryNotice error={model.points.error} onRetry={model.points.refetch} />}
            {model.products.isError && <QueryNotice error={model.products.error} onRetry={model.products.refetch} />}
            <div className={styles.existing}>
                <FormField label="Открыть заказ">
                    <select value={model.mode === 'order' ? model.orderId : ''} disabled={busy}
                        onChange={(event) => model.setOrderIdForView(event.currentTarget.value)}>
                        <option value="">Выберите заказ</option>
                        {externalOrder && <option value={model.orderId}>{model.orderId}</option>}
                        {orders.map(({ id }) => <option key={id} value={id}>{id}</option>)}
                    </select>
                </FormField>
                <Pagination cursor={model.ordersPage.cursor} nextCursor={model.orders.currentData?.nextCursor} busy={busy || model.orders.isFetching}
                    onPrevious={model.ordersPage.previous} onNext={model.ordersPage.next} />
                {model.orders.isError && <QueryNotice error={model.orders.error} onRetry={model.orders.refetch} />}
            </div>
        </div>
    );
}
