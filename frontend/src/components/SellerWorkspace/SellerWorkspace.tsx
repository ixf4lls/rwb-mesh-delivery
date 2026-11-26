import { sellerPanels, useSellerWorkspace } from '../../hooks/useSellerWorkspace';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { SellerCatalog } from '../SellerCatalog/SellerCatalog';
import { SellerFinance } from '../SellerFinance/SellerFinance';
import { SellerOrders } from '../SellerOrders/SellerOrders';
import { SellerStock } from '../SellerStock/SellerStock';
import styles from './SellerWorkspace.module.css';

const panels = Object.keys(sellerPanels) as (keyof typeof sellerPanels)[];

export function SellerWorkspace() {
    const view = useSellerWorkspace();
    const scope = { sellerId: view.sellerId, demoRunId: view.demoRunId };
    const key = `${view.demoRunId ?? ''}:${view.sellerId}`;

    return (
        <div className={styles.workspace}>
            {view.demo.isError && <QueryNotice error={view.demo.error} onRetry={view.demo.refetch} />}
            {view.demo.isLoading && <QueryNotice message="Загружаем контекст демонстрации…" />}
            {view.scopeReady && <>
                <div className={styles.profile}>
                    <form className={styles.form} onSubmit={view.applySeller}>
                        <FormField label="ID продавца">
                            <input key={view.sellerId} name="sellerId" list={view.profileListId} maxLength={128} required
                                pattern="[A-Za-z0-9_\-]+" placeholder="Например, seller-12" defaultValue={view.sellerId} />
                        </FormField>
                        <datalist id={view.profileListId}>
                            {view.sellerIds.map((id) => <option key={id} value={id} />)}
                        </datalist>
                        <Button variant="secondary" type="submit">Открыть профиль</Button>
                    </form>
                    {view.discovery.hasNextPage && <Button variant="secondary" disabled={view.discovery.isFetching}
                        onClick={() => void view.discovery.fetchNextPage()}>Загрузить ещё профили</Button>}
                </div>
                {!view.sellerId && view.discovery.isError && <QueryNotice error={view.discovery.error} onRetry={view.discovery.refetch} />}
                {!view.sellerId && view.discovery.isLoading && <QueryNotice message="Загружаем профили продавцов…" />}
                {!view.sellerId && view.discovery.currentData && <QueryNotice message="Укажите ID продавца, чтобы открыть его каталог, остатки и отчёты." />}
                {view.sellerId && <>
                    <div className={styles.switcher} role="group" aria-label="Разделы кабинета продавца">
                        {panels.map((panel) => <Button key={panel} variant={view.panel === panel ? 'primary' : 'secondary'}
                            aria-pressed={view.panel === panel} onClick={() => view.selectPanel(panel)}>{sellerPanels[panel]}</Button>)}
                    </div>
                    {view.panel === 'catalog' && <SellerCatalog key={key} {...scope} />}
                    {view.panel === 'stock' && <SellerStock key={key} {...scope} />}
                    {view.panel === 'orders' && <SellerOrders key={key} {...scope} />}
                    {view.panel === 'economics' && <SellerFinance key={key} {...scope} clock={view.demo.currentData?.clock} />}
                </>}
            </>}
        </div>
    );
}
