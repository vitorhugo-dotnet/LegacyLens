import { defineConfig } from 'wxt';

export default defineConfig({
  manifest: {
    name: 'LegacyLens',
    description: 'Select and capture PrimeFaces interactions for local investigation.',
    version: '1.0.0',
    permissions: ['nativeMessaging', 'storage', 'scripting', 'activeTab', 'tabs'],
    optional_host_permissions: ['http://*/*', 'https://*/*'],
    action: { default_title: 'LegacyLens capture' },
  },
});
