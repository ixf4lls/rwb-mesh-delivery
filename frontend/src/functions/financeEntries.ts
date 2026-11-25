import type { FinanceParty, FinancePeriod, LedgerEntry } from '../api/analyticsApi';
import type { Order } from '../api/pvzApi';

export const financeParties = { pvz: 'ПВЗ', seller: 'Продавец', marketplace: 'Маркетплейс' } satisfies Record<FinanceParty, string>;

const ledgerNames: Record<LedgerEntry['category'], string> = {
    sale: 'Продажа', delivery_fee: 'Плата за доставку', commission: 'Комиссия', storage_fee: 'Хранение', operation_reward: 'Вознаграждение за обработку',
    logistics_cost: 'Логистика', operating_cost: 'Операционные расходы', cost_of_goods: 'Себестоимость', return_adjustment: 'Корректировка возврата',
    penalty: 'Штраф', bonus: 'Бонус',
};

const periodDate = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeZone: 'UTC' });

export function formatFinancePeriod(period: FinancePeriod) {
    const lastDay = new Date(Date.parse(period.to) - 1);
    return periodDate.formatRange(new Date(period.from), lastDay);
}

export function financeCategory(category: string) {
    return Object.hasOwn(ledgerNames, category) ? ledgerNames[category as LedgerEntry['category']] : category;
}

function basketQuantities(order: Order) {
    const quantities = new Map<string, number>();
    for (const { sellerId, productId, quantity } of order.lines) {
        const key = JSON.stringify([sellerId, productId]);
        quantities.set(key, (quantities.get(key) ?? 0) + quantity);
    }
    return quantities;
}

export function canCompareOrders(mesh: Order, hub: Order) {
    if (mesh.id === hub.id || mesh.pickupPointId !== hub.pickupPointId) return false;

    const meshBasket = basketQuantities(mesh);
    const hubBasket = basketQuantities(hub);
    return meshBasket.size === hubBasket.size && [...meshBasket].every(([key, quantity]) => hubBasket.get(key) === quantity);
}

export function elapsedMinutes(start: string, end?: string) {
    if (!end) return undefined;

    const elapsed = (Date.parse(end) - Date.parse(start)) / 60000;
    return Number.isFinite(elapsed) && elapsed >= 0 ? elapsed : undefined;
}
