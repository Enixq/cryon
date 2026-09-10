import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.cryon.app',
  appName: 'Cryon',
  webDir: 'dist',
  server: {
    // http (а не https): страница приложения тогда имеет origin http://localhost,
    // и обращение к серверу Cryon по http://<IP-ПК>:8899 не блокируется как
    // "mixed content". Кросс-доменность закрывают заголовки CORS сервера.
    // Требует android:usesCleartextTraffic="true" в AndroidManifest.xml.
    androidScheme: 'http'
  }
};

export default config;
