import box from '../../assets/icons/box.svg';
import calendar from '../../assets/icons/calendar.svg';
import dashboard from '../../assets/icons/dashboard.svg';
import profile from '../../assets/icons/profile.svg';
import pvz from '../../assets/icons/pvz.svg';
import routing from '../../assets/icons/routing.svg';
import search from '../../assets/icons/search.svg';
import seller from '../../assets/icons/seller.svg';
import settings from '../../assets/icons/settings.svg';
import styles from './Icon.module.css';

const icons = { box, calendar, dashboard, profile, pvz, routing, search, seller, settings };

type IconProps = {
    name: keyof typeof icons;
    className?: string;
};

export function Icon({ name, className }: IconProps) {
    const classes = [styles.icon, name === 'routing' && styles.routing, className].filter(Boolean).join(' ');

    return <img className={classes} src={icons[name]} alt="" />;
}
