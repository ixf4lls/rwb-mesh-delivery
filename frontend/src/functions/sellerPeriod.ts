const day = 24 * 60 * 60 * 1000;

export function defaultSellerPeriod(now: string | undefined) {
    const reference = now ? Date.parse(now) : Date.now();
    const end = new Date(reference);
    const start = new Date(reference - 29 * day);
    return { start: start.toISOString().slice(0, 10), end: end.toISOString().slice(0, 10) };
}

export function toApiPeriod(start: string, end: string) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(start) || !/^\d{4}-\d{2}-\d{2}$/.test(end)) return undefined;

    const from = Date.parse(`${start}T00:00:00Z`);
    const lastDay = Date.parse(`${end}T00:00:00Z`);
    if (!Number.isFinite(from) || !Number.isFinite(lastDay) || from > lastDay) return undefined;

    return { from: new Date(from).toISOString(), to: new Date(lastDay + day).toISOString() };
}
