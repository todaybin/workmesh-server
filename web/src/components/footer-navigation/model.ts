export const footerNavigationKeys = ['learnMore', 'forum', 'documentation', 'project'] as const;

export type FooterNavigationKey = (typeof footerNavigationKeys)[number];

export interface FooterNavigationLinkSetting {
    visible: boolean;
    url: string;
}

export type FooterNavigationLinks = Record<FooterNavigationKey, FooterNavigationLinkSetting>;

export interface FooterNavigationSetting {
    customized: boolean;
    links: FooterNavigationLinks;
}

export interface FooterNavigationSettingEditor {
    validate: () => Promise<boolean>;
    save: () => Promise<void>;
    restoreDefaults: () => Promise<void>;
    reload: () => Promise<boolean>;
    isDirty: () => boolean;
}

export const createDefaultFooterNavigationLinks = (): FooterNavigationLinks => ({
    learnMore: {
        visible: false,
        url: 'https://www.zoomtk.com/workmesh',
    },
    forum: {
        visible: true,
        url: 'https://www.zoomtk.com/community',
    },
    documentation: {
        visible: false,
        url: 'https://www.zoomtk.com/docs',
    },
    project: {
        visible: true,
        url: 'https://www.zoomtk.com/workmesh',
    },
});

const controlCharacterPattern = /[\u0000-\u001f\u007f-\u009f]/u;
const schemeAuthorityPattern = /^[a-z][a-z\d+.-]*:\/\/([^/?#]*)/i;
const percentBytePattern = /^[\da-f]{2}$/i;

const containsControlCharacter = (value: string) => controlCharacterPattern.test(value);

const decodePercentEscapes = (value: string): string | null => {
    if (!value.includes('%')) {
        return value;
    }

    const encoder = new TextEncoder();
    const bytes: number[] = [];
    let segmentStart = 0;
    for (let index = 0; index < value.length; index += 1) {
        if (value[index] !== '%') {
            continue;
        }
        bytes.push(...encoder.encode(value.slice(segmentStart, index)));
        const escapedByte = value.slice(index + 1, index + 3);
        if (!percentBytePattern.test(escapedByte)) {
            return null;
        }
        bytes.push(Number.parseInt(escapedByte, 16));
        index += 2;
        segmentStart = index + 1;
    }
    bytes.push(...encoder.encode(value.slice(segmentStart)));
    return new TextDecoder().decode(Uint8Array.from(bytes));
};

export const isSafeExternalUrl = (value: unknown): value is string => {
    if (typeof value !== 'string') {
        return false;
    }
    if (containsControlCharacter(value)) {
        return false;
    }

    const normalizedURL = value.trim();
    if (normalizedURL.includes('\\')) {
        return false;
    }
    const authority = normalizedURL.match(schemeAuthorityPattern)?.[1];
    if (!authority || authority.endsWith(':') || authority.includes('@') || authority.includes('%')) {
        return false;
    }

    const decodedURL = decodePercentEscapes(normalizedURL);
    if (decodedURL === null || containsControlCharacter(decodedURL)) {
        return false;
    }

    try {
        const url = new URL(normalizedURL);
        const port = url.port ? Number(url.port) : null;
        return (
            (url.protocol === 'http:' || url.protocol === 'https:') &&
            Boolean(url.hostname) &&
            !url.username &&
            !url.password &&
            (port === null || (Number.isInteger(port) && port >= 1 && port <= 65535))
        );
    } catch {
        return false;
    }
};

const legacyPanelHosts = new Set(['workmesh.cn', 'workmesh.pro']);

const isLegacyPanelUrl = (value: string): boolean => {
    try {
        const parsed = new URL(value);
        const hostname = parsed.hostname.toLowerCase();
        return (
            legacyPanelHosts.has(hostname) ||
            hostname.endsWith('.workmesh.cn') ||
            hostname.endsWith('.workmesh.pro') ||
            (hostname === 'github.com' && parsed.pathname.toLowerCase().startsWith('/workmesh-dev/workmesh'))
        );
    } catch {
        return false;
    }
};

export const mergeFooterNavigationLinks = (
    setting: FooterNavigationSetting | null,
    defaults: FooterNavigationLinks,
): FooterNavigationLinks => {
    if (!setting?.customized || !setting.links) {
        return defaults;
    }

    return footerNavigationKeys.reduce((links, key) => {
        const customized = setting.links[key];
        links[key] = {
            visible: typeof customized?.visible === 'boolean' ? customized.visible : defaults[key].visible,
            url:
                isSafeExternalUrl(customized?.url) && !isLegacyPanelUrl(customized.url)
                    ? customized.url
                    : defaults[key].url,
        };
        return links;
    }, {} as FooterNavigationLinks);
};
