export const workspaceRoutes = [
    {
        path: '/pvz',
        title: 'Кабинет ПВЗ',
        shortTitle: 'ПВЗ',
        icon: 'pvz',
        description: 'Приёмка партий, хранение, сборка и выдача заказов.',
    },
    {
        path: '/seller',
        title: 'Кабинет продавца',
        shortTitle: 'Продавец',
        icon: 'seller',
        description: 'Товары, партии, остатки и стоимость хранения.',
    },
    {
        path: '/delivery-model',
        title: 'Модель доставки',
        shortTitle: 'Модель',
        icon: 'routing',
        description: 'Маршруты, сроки и сравнение Mesh с доставкой через хаб.',
    },
] as const;

export const overviewRoute = { path: '/', title: 'Обзор', shortTitle: 'Обзор', icon: 'dashboard' } as const;

export const navigationRoutes = [overviewRoute, workspaceRoutes[2], workspaceRoutes[0], workspaceRoutes[1]];

export const demoRoles = [
    { id: 'pvz_operator', title: 'Оператор ПВЗ', workspace: workspaceRoutes[0] },
    { id: 'seller', title: 'Продавец', workspace: workspaceRoutes[1] },
    { id: 'dispatcher', title: 'Диспетчер', workspace: workspaceRoutes[2] },
] as const;

export type DemoRole = (typeof demoRoles)[number];
