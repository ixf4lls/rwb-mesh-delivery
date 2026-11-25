import { formatMoney } from '../../functions/money';
import { useSellerCatalog } from '../../hooks/useSellerCatalog';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './SellerCatalog.module.css';

const moneyPattern = '[0-9]+([,.][0-9]{1,2})?';

export function SellerCatalog({ sellerId, demoRunId }: { sellerId: string; demoRunId?: string }) {
    const view = useSellerCatalog(sellerId, demoRunId);
    const { catalog, creation } = view;
    const rows = catalog.currentData?.pages.flatMap(({ items }) => items.map((product) => ({
        id: product.id,
        cells: [<div><strong>{product.name}</strong><small>{product.id}</small></div>, product.sellerSku,
            product.category || '—', `${product.weightGrams} г`, product.slotsPerUnit, formatMoney(product.salePriceMinor),
            product.unitCostMinor === undefined ? '—' : formatMoney(product.unitCostMinor)],
    }))) ?? [];

    return (
        <div className={styles.catalog}>
            <div className={styles.actions}><Button onClick={view.openForm}>Зарегистрировать товар</Button></div>
            {view.success && <QueryNotice message={view.success} />}
            {catalog.isError && <QueryNotice error={catalog.error} onRetry={catalog.refetch} />}
            {catalog.isLoading && <QueryNotice message="Загружаем товары…" />}
            {catalog.currentData && <DataTable title="Каталог товаров"
                columns={['Товар', 'Артикул', 'Категория', 'Вес', 'Мест на штуку', 'Цена', 'Себестоимость']}
                rows={rows} emptyMessage="У этого продавца пока нет товаров." />}
            {catalog.hasNextPage && <div className={styles.actions}>
                <Button variant="secondary" disabled={catalog.isFetching} onClick={() => void catalog.fetchNextPage()}>Загрузить ещё товары</Button>
            </div>}
            <Dialog title="Регистрация товара" open={view.open} busy={creation.pending} onClose={view.closeForm}>
                <form className={styles.form} onSubmit={(event) => void view.submitProduct(event)}>
                    <p>Продавец <strong>{sellerId}</strong></p>
                    <fieldset disabled={creation.pending}>
                        <FormField label="Название"><input name="name" required maxLength={200} /></FormField>
                        <FormField label="Артикул"><input name="sellerSku" required maxLength={128} /></FormField>
                        <FormField label="Категория (необязательно)"><input name="category" maxLength={100} /></FormField>
                        <div className={styles.fields}>
                            <FormField label="Вес, г"><input type="number" name="weightGrams" min={1} step={1} required /></FormField>
                            <FormField label="Мест на штуку"><input type="number" name="slotsPerUnit" min={1} step={1} required /></FormField>
                            <FormField label="Цена продажи, ₽"><input name="salePrice" inputMode="decimal" pattern={moneyPattern} required /></FormField>
                            <FormField label="Себестоимость, ₽"><input name="unitCost" inputMode="decimal" pattern={moneyPattern} required /></FormField>
                        </div>
                    </fieldset>
                    {(view.validation || creation.error) && <p className={styles.error} role="alert">{view.validation || creation.error}</p>}
                    <div className={styles.actions}>
                        <Button type="submit" disabled={creation.pending}>
                            {creation.pending ? 'Сохраняем…' : creation.error ? 'Повторить' : 'Зарегистрировать'}
                        </Button>
                        <Button variant="secondary" disabled={creation.pending} onClick={view.closeForm}>Закрыть</Button>
                    </div>
                </form>
            </Dialog>
        </div>
    );
}
