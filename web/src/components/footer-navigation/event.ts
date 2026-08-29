export const FOOTER_NAVIGATION_REFRESH_EVENT = 'workmesh:footer-navigation-refresh';

export const refreshFooterNavigation = () => {
    window.dispatchEvent(new Event(FOOTER_NAVIGATION_REFRESH_EVENT));
};
