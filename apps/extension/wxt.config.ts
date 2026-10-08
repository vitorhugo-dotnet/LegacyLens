import { defineConfig } from 'wxt';

export default defineConfig({
  manifest: {
    ...(process.env.LEGACYLENS_FIXTURE_EXTENSION === '1' ? { key: 'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArInlyXeSU4y2YpuqX8wGzGQXboKazCL2kpiYA/J8UdY3qExXE2lur+CC55xr+osXJ9ELS4PqQynWBLOnNAuW/H4lOXC7d82TXrBuxpLKlRC/YuOLhYrdjtNbnbtQnQGRu7n2GUkFKpBnDyePRum4hB7VLnemfytVXgk+WfxzbLS3PPrN71iIsGg9QLVTbBu/iXZNO7S1r/QJjMFzeKVX0tA4SrqG05xnxJQV9GCEpqKiL2IUJuhKQ8q9JBXDQPUfHlXsE7umCoUXuVkQmQ7XsGn5MGOA71vWv4pjPGGzhwN44x4J2Z5GEhS6yGvG86vg+s71PFbMFdPOqqqSnYCWdQIDAQAB' } : {}),
    name: 'LegacyLens',
    description: 'Select and capture PrimeFaces interactions for local investigation.',
    version: '1.0.0',
    permissions: ['nativeMessaging', 'storage', 'scripting', 'activeTab', 'tabs', 'contextMenus'],
    optional_host_permissions: ['http://*/*', 'https://*/*'],
    ...(process.env.LEGACYLENS_FIXTURE_EXTENSION === '1' ? { host_permissions: ['http://127.0.0.1/*'] } : {}),
    action: { default_title: 'LegacyLens capture' },
  },
});
