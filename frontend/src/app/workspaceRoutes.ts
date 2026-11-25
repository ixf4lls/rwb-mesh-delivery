export const workspaceRoutes = [
    {
        path: '/pvz',
        title: 'Кабинет ПВЗ',
        shortTitle: 'ПВЗ',
        icon: 'pvz',
        description: 'Приёмка партий, хранение, сборка и выдача заказов.',
        contentTitle: 'Очередь задач',
        columns: ['Задача', 'Партия или заказ', 'Статус'],
        emptyMessage: 'Здесь появятся задачи по приёмке, сборке и выдаче заказов.',
    },
    {
        path: '/seller',
        title: 'Кабинет продавца',
        shortTitle: 'Продавец',
        icon: 'seller',
        description: 'Товары, партии, остатки и стоимость хранения.',
        contentTitle: 'Товары и остатки',
        columns: ['Товар', 'Место хранения', 'Доступно', 'В резерве'],
        emptyMessage: 'Здесь появятся товары, партии и доступные остатки.',
    },
    {
        path: '/delivery-model',
        title: 'Модель доставки',
        shortTitle: 'Модель',
        icon: 'routing',
        description: 'Маршруты, сроки и сравнение Mesh с доставкой через хаб.',
        contentTitle: 'Логистическая сеть',
        emptyMessage: 'Здесь появятся узлы сети и маршруты для выбранного заказа.',
    },
] as const;

export type Workspace = (typeof workspaceRoutes)[number];

export const overviewRoute = { path: '/', title: 'Обзор', shortTitle: 'Обзор', icon: 'dashboard' } as const;

export const navigationRoutes = [overviewRoute, workspaceRoutes[2], workspaceRoutes[0], workspaceRoutes[1]];

export const demoRoles = [
    { id: 'pvz_operator', title: 'Оператор ПВЗ', workspace: workspaceRoutes[0] },
    { id: 'seller', title: 'Продавец', workspace: workspaceRoutes[1] },
    { id: 'dispatcher', title: 'Диспетчер', workspace: workspaceRoutes[2] },
] as const;

export type DemoRole = (typeof demoRoles)[number];
