const rubles = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' });

export function formatMoney(minor: number) {
    return rubles.format(minor / 100);
}

export function parseRubles(value: string) {
    const parts = /^(\d+)(?:[,.](\d{1,2}))?$/.exec(value.trim());
    if (!parts) return undefined;

    const amount = Number(parts[1]) * 100 + Number((parts[2] ?? '').padEnd(2, '0'));
    return Number.isSafeInteger(amount) ? amount : undefined;
}
