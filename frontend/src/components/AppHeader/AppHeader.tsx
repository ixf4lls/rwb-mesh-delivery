import { Link } from 'react-router';
import type { DemoRole } from '../../app/workspaceRoutes';
import { useWorkspaceSearch } from '../../hooks/useWorkspaceSearch';
import { Button } from '../Button/Button';
import { Icon } from '../Icon/Icon';
import { SearchField } from '../SearchField/SearchField';
import styles from './AppHeader.module.css';

type AppHeaderProps = {
    currentRole: DemoRole;
    onOpenRoleDialog: () => void;
};

export function AppHeader({ currentRole, onOpenRoleDialog }: AppHeaderProps) {
    const search = useWorkspaceSearch();

    return (
        <header className={styles.header}>
            <form className={styles.search} role="search" onSubmit={search.goToFirstResult}
                onFocus={search.handleFocus} onBlur={search.handleBlur} onKeyDown={search.handleEscape}>
                <SearchField name="workspace" value={search.query} onChange={(event) => search.setQuery(event.target.value)}
                    aria-label="Поиск по разделам" placeholder="Поиск по разделам" autoComplete="off"
                    submitLabel="Открыть первый найденный раздел" />
                {search.showResults && (
                    <div className={styles.results}>
                        {search.results.length > 0 ? (
                            <ul aria-label="Результаты поиска">
                                {search.results.map(({ path, title, icon }) => (
                                    <li key={path}>
                                        <Link to={path} onClick={search.clearSearch}>
                                            <Icon name={icon} />
                                            {title}
                                        </Link>
                                    </li>
                                ))}
                            </ul>
                        ) : <p role="status">Разделы не найдены</p>}
                    </div>
                )}
            </form>
            <Button variant="secondary" className={styles.role} onClick={onOpenRoleDialog} aria-haspopup="dialog"
                aria-label={`Сменить демонстрационную роль. Текущая роль: ${currentRole.title}`}>
                <Icon name="settings" className={styles.settingsIcon} />
                <span className={styles.roleLabel}>
                    <span className={styles.caption}>Деморежим</span>
                    <span>{currentRole.title}</span>
                </span>
            </Button>
        </header>
    );
}
