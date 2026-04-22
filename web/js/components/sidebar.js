/**
 * 侧边栏导航组件 — 内联 SVG 图标
 */
const AppSidebar = {
    template: `
    <nav class="sidebar">
        <div class="sidebar-logo">C</div>
        <div class="sidebar-nav">
            <router-link 
                v-for="item in navItems" 
                :key="item.path"
                :to="item.path" 
                class="sidebar-item"
                active-class="active"
            >
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-html="item.svg"></svg>
                <span class="tooltip">{{ $t(item.labelKey) }}</span>
            </router-link>
        </div>
    </nav>
    `,
    data() {
        return {
            navItems: [
                { 
                    path: '/', 
                    labelKey: 'nav.dashboard',
                    svg: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>'
                },
                { 
                    path: '/services', 
                    labelKey: 'nav.services',
                    svg: '<path d="M22 8.5c0-.28-.22-.5-.5-.5h-19c-.28 0-.5.22-.5.5v11c0 .28.22.5.5.5h19c.28 0 .5-.22.5-.5V8.5z"/><path d="M7 8V6a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v2"/><line x1="12" y1="12" x2="12" y2="16"/><line x1="8" y1="12" x2="8" y2="16"/><line x1="16" y1="12" x2="16" y2="16"/>'
                },
                { 
                    path: '/logs', 
                    labelKey: 'nav.logs',
                    svg: '<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/>'
                },
                { 
                    path: '/env', 
                    labelKey: 'nav.env',
                    svg: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>'
                }
            ]
        };
    }
};
