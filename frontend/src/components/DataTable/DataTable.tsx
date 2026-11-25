import type { ReactNode } from 'react';
import styles from './DataTable.module.css';

type DataTableProps = {
    title: string;
    columns: readonly string[];
    rows: readonly { id: string; cells: readonly ReactNode[] }[];
    emptyMessage: string;
};

export function DataTable({ title, columns, rows, emptyMessage }: DataTableProps) {
    return (
        <div className={styles.panel}>
            <h2 className={styles.title}>{title}</h2>
            <div className={styles.scroll}>
                <table className={styles.table} aria-label={title}>
                    <thead>
                        <tr>{columns.map((column) => <th key={column} scope="col">{column}</th>)}</tr>
                    </thead>
                    <tbody>
                        {rows.length > 0 ? rows.map(({ id, cells }) => (
                            <tr key={id}>
                                {columns.map((column, index) => <td key={column}>{cells[index]}</td>)}
                            </tr>
                        )) : (
                            <tr>
                                <td className={styles.empty} colSpan={columns.length}>{emptyMessage}</td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
