// Service Worker for Jonathan & Julene Wedding Platform
const CACHE_NAME = 'wedding-v2.0';
const STATIC_ASSETS = [
  '/',
  '/invite',
  '/photos',
  '/gallery'
];

self.addEventListener('install', (e) => {
  self.skipWaiting();
  e.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(STATIC_ASSETS)).catch(() => {})
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))
      );
    })
  );
  return self.clients.claim();
});

// Web Push Notification Event Listener
self.addEventListener('push', (e) => {
  let payload = {
    title: 'Jonathan & Julene Troue • Kennisgewing',
    body: 'Nuwe troue opdatering beskikbaar!',
    url: '/photos'
  };

  if (e.data) {
    try {
      payload = e.data.json();
    } catch (err) {
      payload.body = e.data.text();
    }
  }

  const options = {
    body: payload.body,
    icon: '/wedding_qr.png',
    badge: '/wedding_qr.png',
    data: {
      url: payload.url || '/photos'
    },
    vibrate: [200, 100, 200]
  };

  e.waitUntil(
    self.registration.showNotification(payload.title, options)
  );
});

// Notification Click Handler
self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const targetUrl = (e.notification.data && e.notification.data.url) ? e.notification.data.url : '/photos';

  e.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
      for (const client of clientList) {
        if (client.url.includes(targetUrl) && 'focus' in client) {
          return client.focus();
        }
      }
      if (clients.openWindow) {
        return clients.openWindow(targetUrl);
      }
    })
  );
});
